package main

import (
	"github.com/rz1998/invest-md-uju/rpc/mdCurrent"
	tradeBasic "github.com/rz1998/invest-trade-basic"
	"github.com/rz1998/invest-trade-basic/demoTradeSpi"
	"time"
)

func main() {
	api := mdCurrent.ApiMDUju{}
	var spi tradeBasic.ISpiMD
	spi = &demoTradeSpi.SpiMDPrint{}
	api.SetSpi(&spi)
	api.Login(nil)
	time.Sleep(5 * time.Second)
	api.Sub([]string{"000001.SSE"})
	select {}
}
