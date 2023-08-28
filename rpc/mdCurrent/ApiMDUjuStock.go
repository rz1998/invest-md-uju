package mdCurrent

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/rz1998/invest-basic/types/investBasic"
	"github.com/rz1998/invest-md-uju/rpc/mdCurrent/internal/config"
	"github.com/rz1998/invest-md-uju/rpc/mdCurrent/internal/logic"
	"github.com/rz1998/invest-md-uju/rpc/mdCurrent/internal/logic/snappy"
	"github.com/rz1998/invest-md-uju/rpc/mdCurrent/types/mdCurrent"
	trade "github.com/rz1998/invest-trade-basic"
	"github.com/rz1998/invest-trade-basic/types/tradeBasic"
	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
)

type ApiMDUju struct {
	conf      *config.Config
	spi       *trade.ISpiMD
	cnIn      chan *mdCurrent.NettyMessage
	cnOut     chan *mdCurrent.NettyMessage
	conn      *net.Conn
	needLogin bool
	isLogin   bool
	subCodes  []string
	infoAc    *tradeBasic.PInfoAc
}

func (api *ApiMDUju) SetSpi(spi *trade.ISpiMD) {
	api.spi = spi
}

func (api *ApiMDUju) GetSpi() *trade.ISpiMD {
	return api.spi
}

func (api *ApiMDUju) Login(infoAc *tradeBasic.PInfoAc) {
	api.infoAc = infoAc
	api.needLogin = true
	// 加载配置文件
	var configFile = flag.String("f", "etc/mdCurrent.yaml", "the config file")
	flag.Parse()
	api.conf = &config.Config{}
	conf.MustLoad(*configFile, &api.conf, conf.UseEnv())
	// 初始化
	api.cnIn = make(chan *mdCurrent.NettyMessage, 10)
	api.cnOut = make(chan *mdCurrent.NettyMessage, 10)
	// 连接
	var conn net.Conn
	var err error
	ipSvr := api.conf.Target
	if infoAc != nil && infoAc.BrokerId != "" {
		ipSvr = infoAc.BrokerId
	}
	if conn, err = net.Dial("tcp", ipSvr); err != nil {
		time.Sleep(2 * time.Second)
		api.Login(api.infoAc)
		return
	}
	api.conn = &conn
	logx.Info("connect", ipSvr, "success")
	// 消息读取
	go readFromService(api)
	// 消息处理
	go handleMessage(api)
	// 发送登录请求
	api.sendMsgLogin()
}

func write2Service(api *ApiMDUju) {
	for msg := range api.cnOut {
		if !api.isLogin {
			break
		}
		data, _ := encode(msg)
		_, err := (*api.conn).Write(data)
		if err != nil {
			logx.Error(fmt.Sprintf("write2Service send error %v", err))
			break
		}

		// log
		//fmt.Printf("sending %v\n", data)

	}
	if api.needLogin {
		api.Login(api.infoAc)
	}
}

func readFromService(api *ApiMDUju) {
	cacheSize := api.conf.MDUju.CatchSize
	for {
		// 解析msg
		buf := make([]byte, api.conf.MDUju.CatchSize)
		n, err := (*api.conn).Read(buf)
		if err != nil {
			if err == io.EOF {
				// 结束
				logx.Error(fmt.Sprintf("readMessage read eof %T %v", err, err))
			} else if err == io.ErrClosedPipe {
				logx.Error(fmt.Sprintf("readMessage read stopped %T %v", err, err))
			} else if strings.Contains(err.Error(), "closed") {
				// 重连
				logx.Info(fmt.Sprintf("readMessage read closed %T %v", err, err))
				logx.Info(fmt.Sprintf("readMessage need reconnect:%t", api.needLogin))
				api.isLogin = false
			} else {
				logx.Info(fmt.Sprintf("readMessage read error %T %v", err, err))
			}
			break
		}
		if n == 0 {
			continue
		}
		api.cnIn <- decode(buf, n, cacheSize)
	}
	if api.needLogin {
		api.Login(api.infoAc)
	}
}

func (api *ApiMDUju) sendMsgLogin() {
	msg := &mdCurrent.NettyMessage{
		Header: &mdCurrent.Header{
			Type: mdCurrent.Header_LOGIN_REQ,
		},
		LoginReqMessage: &mdCurrent.LoginReqMessage{
			Username: api.conf.MDUju.Username,
			Passwd:   logic.Md5V(api.conf.MDUju.Password),
		},
	}
	// 登录消息是直接发送的
	data, _ := encode(msg)
	_, err := (*api.conn).Write(data)
	if err != nil {
		logx.Error(fmt.Sprintf("send error %v", err))
	}
}

func sendMsgHeartBeat(api *ApiMDUju) {
	for range time.Tick(5 * time.Second) {
		if !api.isLogin {
			break
		}
		msg := &mdCurrent.NettyMessage{
			Header: &mdCurrent.Header{
				Type: mdCurrent.Header_HEARTBEAT_REQ,
			},
		}
		api.cnOut <- msg
	}
}

func (api *ApiMDUju) Logout() {
	api.needLogin = false
	api.subCodes = nil
	logx.Info("closing cn out")
	close(api.cnOut)
	logx.Info("closing cn in")
	close(api.cnIn)
	logx.Info("closing conn")
	if api.conn != nil {
		err := (*api.conn).Close()
		logx.Info(fmt.Sprintf("ApiMDUjuStock logout %v", err))
	}
	logx.Info("ApiMDUjuStock finished logout")
}

func (api *ApiMDUju) Sub(uniqueCodes []string) {
	if uniqueCodes == nil || len(uniqueCodes) == 0 {
		return
	}
	if !api.IsLogin() {
		logx.Error(fmt.Sprintf("%s stopped by %s", "Sub", "not login"))
		return
	}
	// 去重
	mapCode := make(map[string]byte)
	if api.subCodes != nil && len(api.subCodes) > 0 {
		for _, code := range api.subCodes {
			mapCode[code] = 0
		}
	}
	// 订阅去重之后的
	var ujuCodes []string
	for _, uniqueCode := range uniqueCodes {
		if _, ok := mapCode[uniqueCode]; !ok {
			ujuCodes = append(ujuCodes, logic.FromStd2UjuUniqueCode(uniqueCode))
			mapCode[uniqueCode] = 0
		}
	}
	if ujuCodes != nil && len(ujuCodes) > 0 && api.cnOut != nil {
		logx.Error(fmt.Sprintf("subing %v", ujuCodes))
		msg := &mdCurrent.NettyMessage{
			Header: &mdCurrent.Header{
				Type: mdCurrent.Header_SERVICE_REQ,
			},
			BusinessReqMessage: &mdCurrent.BusinessReqMessage{
				StockCodes:   ujuCodes,
				Types:        []int32{-91, -92, -89, -87},
				BusinessType: 1,
			},
		}
		api.cnOut <- msg
		// 保存
		api.subCodes = make([]string, len(mapCode))
		i := 0
		for k := range mapCode {
			api.subCodes[i] = k
			i++
		}
	}
}

func (api *ApiMDUju) Unsub(uniqueCodes []string) {
	//if uniqueCodes == nil || len(uniqueCodes) == 0 {
	//	return
	//}
	//ujuCodes := make([]string, len(uniqueCodes))
	//for i, uniqueCode := range uniqueCodes {
	//	ujuCodes[i] = FromStd2UjuUniqueCode(uniqueCode)
	//}
	//msg := &service.NettyMessage{
	//	Header: &service.Header{
	//		Type: service.Header_SERVICE_REQ,
	//	},
	//	BusinessReqMessage: &service.BusinessReqMessage{
	//		StockCodes:   ujuCodes,
	//		Types:        []int32{-91, -92, -89, -87},
	//		BusinessType: 1,
	//	},
	//}
	//a.cnOut <- msg
}

func (api *ApiMDUju) GetInfoAc() *tradeBasic.PInfoAc {
	return nil
}

func (api *ApiMDUju) IsLogin() bool {
	return api.isLogin
}

func (api *ApiMDUju) SetIsLogin(isLogin bool) {
	api.isLogin = isLogin
}

func handleMessage(api *ApiMDUju) {
	cnIn := api.cnIn
	spi := api.spi
	for msg := range cnIn {
		if msg == nil || msg.Header == nil {
			continue
		}
		// 跳过心跳
		if msg.Header.Type == mdCurrent.Header_HEARTBEAT_RESP {
			continue
		}
		if msg.Header.Type == mdCurrent.Header_LOGIN_RESP {
			if msg.LoginRespMessage.LoginResult == mdCurrent.LoginRespMessage_FAIL {
				logx.Error(fmt.Sprintf("login failed %+v", msg))
			} else {
				logx.Info("login succeed...")
				api.isLogin = true
				// 登录成功保持消息发送
				go write2Service(api)
				// 登录成功发送心跳
				go sendMsgHeartBeat(api)
				// 检查如果有订阅股票则重新订阅
				if api.subCodes != nil && len(api.subCodes) > 0 {
					codes := api.subCodes
					api.subCodes = nil
					api.Sub(codes)
				}
			}
		} else if msg.Header.Type == mdCurrent.Header_SERVICE_RESP {
			businessRespList := msg.GetBusinessRespMessage()
			if businessRespList != nil {
				for _, result := range businessRespList {
					if result != nil {
						switch result.GetDataType() {
						case 4:
							// 股票订阅回报
							codes := result.Stocks
							for _, code := range codes {
								(*spi).OnRspSub(code, nil, -1, false)
							}
							(*spi).OnRspSub("", nil, -1, true)
						case -91: // MSG_DATA_MARKET
							// 行情数据
							markets := result.UjuTdfMarketData
							mds := fromUju2StdMDStock(markets)
							if api.spi != nil {
								for _, md := range mds {
									(*spi).OnRtnMD(md)
								}
							}
						case -92: // MSG_DATA_INDEX = -92
							markets := result.GetUjuTdfIndexData()
							mds := fromUju2StdMDIdx(markets)
							if api.spi != nil {
								for _, md := range mds {
									(*spi).OnRtnMD(md)
								}
							}
						}
						// MSG_DATA_TRANSACTION = -89
						// MSG_DATA_ORDER = -87
					}
				}
			}

		}
		//fmt.Printf("%v\n", msg)
	}
}

func fromUju2StdMDIdx(markets []*mdCurrent.UJU_TDF_INDEX_DATA) []*investBasic.SMDTick {
	var mds []*investBasic.SMDTick
	if markets == nil {
		return mds
	}
	mds = make([]*investBasic.SMDTick, len(markets))
	for i, market := range markets {
		dayTrade, _ := time.Parse("20060102", fmt.Sprintf("%d", market.GetTradingDay()))
		strTime := ""
		if market.GetTime() >= 100000000 {
			strTime = fmt.Sprintf("%d", market.GetTime())
		} else {
			strTime = fmt.Sprintf("0%d", market.GetTime())
		}
		timeMD, _ := time.ParseInLocation("20060102 150405000", fmt.Sprintf("%d %s", market.GetTradingDay(), strTime), time.FixedZone("CST", 8*3600))
		timestamp := timeMD.UnixMilli()
		mds[i] = &investBasic.SMDTick{
			UniqueCode:    logic.FromUju2StdUniqueCode(market.GetWindCode()),
			DayTrade:      dayTrade.Format("2006-01-02"),
			Timestamp:     timestamp,
			PricePreClose: market.GetPreCloseIndex(),
			PriceOpen:     market.GetOpenIndex(),
			PriceHigh:     market.GetHighIndex(),
			PriceLow:      market.GetLowIndex(),
			PriceLatest:   market.GetLastIndex(),
			Vol:           market.GetTotalVolume(),
			Val:           float64(market.GetTurnover()),
		}
	}
	return mds
}

func fromUju2StdMDStock(markets []*mdCurrent.UJU_TDF_MARKET_DATA) []*investBasic.SMDTick {
	var mds []*investBasic.SMDTick
	if markets == nil {
		return mds
	}
	mds = make([]*investBasic.SMDTick, len(markets))
	for i, market := range markets {
		dayTrade, _ := time.Parse("20060102", fmt.Sprintf("%d", market.GetTradingDay()))
		strTime := ""
		if market.GetTime() >= 100000000 {
			strTime = fmt.Sprintf("%d", market.GetTime())
		} else {
			strTime = fmt.Sprintf("0%d", market.GetTime())
		}
		timeMD, _ := time.ParseInLocation("20060102 150405000", fmt.Sprintf("%d %s", market.GetTradingDay(), strTime), time.FixedZone("CST", 8*3600))
		timestamp := timeMD.UnixMilli()
		mds[i] = &investBasic.SMDTick{
			UniqueCode:          logic.FromUju2StdUniqueCode(market.GetWindCode()),
			DayTrade:            dayTrade.Format("2006-01-02"),
			Timestamp:           timestamp,
			PricePreClose:       market.GetPreClose(),
			PriceLimitLower:     market.GetLowLimited(),
			PriceLimitUpper:     market.GetHighLimited(),
			PriceOpen:           market.GetOpen(),
			PriceHigh:           market.GetHigh(),
			PriceLow:            market.GetLow(),
			PriceLatest:         market.GetMatch(),
			Num:                 market.GetNumTrades(),
			Vol:                 market.GetVolume(),
			Val:                 float64(market.GetTurnover()),
			NegVal:              float64(market.GetIOPV()),
			AskVols:             market.GetBidVol(),
			BidVols:             market.GetAskVol(),
			AskPrices:           market.GetBidPrice(),
			BidPrices:           market.GetAskPrice(),
			WeightedAvgAskPrice: market.GetWeightedAvgBidPrice(),
			WeightedAvgBskPrice: market.GetWeightedAvgAskPrice(),
			AskVolTotal:         market.GetTotalBidVol(),
			BidVolTotal:         market.GetTotalAskVol(),
		}
	}
	return mds
}

func decode(buf []byte, n int, catchSize int32) *mdCurrent.NettyMessage {
	//// 解压
	decoded := make([]byte, catchSize)
	var size uint64
	var index int
	var err error
	r := snappy.NewReader(bytes.NewReader(buf[:n]))
	_, err = r.Read(decoded)
	if err != nil {
		logx.Error(fmt.Sprintf("decode snappy decode error %v", err))
	}
	//decoded := reverse(buf, n)
	size, index = proto.DecodeVarint(decoded)
	// 解析为proto
	test := decoded[index : size+uint64(index)]
	msg := &mdCurrent.NettyMessage{}
	err = proto.Unmarshal(test, msg)
	if err != nil {
		logx.Error(fmt.Sprintf("decode error %v", err))
	}
	return msg
}

// encode 将数据包编码（即加上包头再转为二进制）
func encode(msg *mdCurrent.NettyMessage) ([]byte, error) {
	//protobuf编码
	pData, err := proto.Marshal(msg)
	if err != nil {
		panic(err)
	}
	//获取发送数据的长度，并转换为四个字节的长度，即int32
	size := uint32(len(pData))
	//创建数据包
	dataPackage := new(bytes.Buffer) //使用字节缓冲区，一步步写入性能更高

	//先向缓冲区写入包头
	logic.WriteVaruint32(dataPackage, size)

	// 写入消息1
	dataPackage.Write(pData)

	// snappy压缩
	dataC := &bytes.Buffer{}
	writer := snappy.NewBufferedWriter(dataC)
	writer.Write(dataPackage.Bytes())
	writer.Close()
	return dataC.Bytes(), nil
}
