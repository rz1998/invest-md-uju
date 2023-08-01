package config

import (
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
)

type ConfMDUju struct {
	Username  string
	Password  string
	CatchSize int32
}

type Config struct {
	zrpc.RpcClientConf
	Log   logx.LogConf
	MDUju ConfMDUju
}
