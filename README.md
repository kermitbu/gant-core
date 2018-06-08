# gant-core

## 设计想法
最简单的方法来创建一个节点，每个节点定义一个继承于CoreServer的结构即可。
```go
package main

import (
	"flag"
	core "github.com/kermitbu/gant-core"
)

type MyServer struct {
	core.CoreServer
}


func InitTcpSvr(port string) {
	svr := new(MasterServer)

	// 注册事件处理方法
	svr.Handle(1, func(req *core.GRequest, res *core.GResponse) {
	})
	
	// 注册事件处理方法
	svr.Handle(2, func(req *core.GRequest, res *core.GResponse) {

	})

	// 初始化服务器
	err := svr.InitConnectAsServer(port)
	if err != nil {
		log.Error("%s", err.Error())
	}
}

func main() {
	// 开始监听9687端口
	InitTcpSvr("9687")
}

```


## 协议定义
消息头+自定义数据
```proto
type MessageHead struct {
	Cmd     uint16
	Version byte
	HeadLen byte
	BodyLen uint16
}
```
