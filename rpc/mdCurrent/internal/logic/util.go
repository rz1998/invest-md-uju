package logic

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"github.com/rz1998/invest-basic/types/investBasic"
	tradeBasic "github.com/rz1998/invest-trade-basic"
	"github.com/zeromicro/go-zero/core/logx"
	"io"
)

func FromStd2UjuUniqueCode(uniqueCode string) string {
	code, exchangeCD := tradeBasic.GetSecInfo(uniqueCode)
	if exchangeCD == "" {
		return uniqueCode
	}
	ujuCode := code + "."
	switch exchangeCD {
	case investBasic.SSE:
		ujuCode += "SH"
	case investBasic.SZSE:
		ujuCode += "SZ"
	default:
		logx.Error(fmt.Sprintf("FromStd2UjuUniqueCode unhandled exchangeCD %s", exchangeCD))
	}
	return ujuCode
}

func FromUju2StdUniqueCode(ujuCode string) string {
	code, exchangeCDUju := tradeBasic.GetSecInfo(ujuCode)
	var exchangeCD investBasic.ExchangeCD
	switch string(exchangeCDUju) {
	case "SH":
		exchangeCD = investBasic.SSE
	case "SZ":
		exchangeCD = investBasic.SZSE
	default:
		logx.Error(fmt.Sprintf("FromUju2StdUniqueCode unhandled exchangeCD %s", exchangeCDUju))
	}
	return fmt.Sprintf("%s.%s", code, exchangeCD)
}

func Md5V(str string) string {
	h := md5.New()
	h.Write([]byte(str))
	return hex.EncodeToString(h.Sum(nil))
}

// WriteVaruint32 writes a uint32 to the destination buffer passed with a size of 1-5 bytes.
func WriteVaruint32(dst io.ByteWriter, x uint32) error {
	for x >= 0x80 {
		if err := dst.WriteByte(byte(x) | 0x80); err != nil {
			return err
		}
		x >>= 7
	}
	return dst.WriteByte(byte(x))
}
