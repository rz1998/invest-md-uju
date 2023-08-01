package demo

import (
	"context"
	"flag"
	"fmt"
	"github.com/rz1998/invest-md-uju/rpc/mdHistory/futurehistickservice"
	"github.com/rz1998/invest-md-uju/rpc/mdHistory/types/mdHistoryUju"
	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/zrpc"
)

func main() {
	// conf
	var confClient = &zrpc.RpcClientConf{}
	configFile := flag.String("f", "etc/mdHistory.yaml", "the config file")
	flag.Parse()
	conf.MustLoad(*configFile, confClient, conf.UseEnv())
	//
	conn := zrpc.MustNewClient(*confClient)
	client := futurehistickservice.NewFutureHisTickService(conn)
	rtn, err := client.QueryStockHisTick(context.TODO(), &mdHistoryUju.Request{
		InstrumentId: "600030.SH",
		Date:         "20230712",
		ExchangeId:   "SH",
	})
	if err != nil {
		fmt.Printf("%v\n", err)
		return
	}
	fmt.Printf("%+v\n", rtn)
}
