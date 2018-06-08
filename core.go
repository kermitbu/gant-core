package core

import (
	"errors"
	"io"
	"net"
	"time"

	"github.com/kermitbu/gant-log"
)

// Command 定义命令类型
type Command uint16

type handleFunc func(request *GRequest, response *GResponse)

// BufLength 缓冲区长度
const BufLength = 10240

type CoreServer struct {
	allHandlerFunc map[uint16]handleFunc
	remoteAddr     string
	connect        *net.TCPConn
	ListenPort     string
}

// Handle 外部用于注册事件处理方法的方法
func (c *CoreServer) Handle(id uint16, f handleFunc) {

	if c.allHandlerFunc == nil {
		c.allHandlerFunc = make(map[uint16]handleFunc)
	}
	if _, ok := c.allHandlerFunc[id]; ok {
		log.Warn("Register called twice for handles ", id)
	}
	c.allHandlerFunc[id] = f
}

// 派发事件
func (c *CoreServer) deliverMessage(conn net.Conn, msghead *MessageHead, body []byte) {

	if handler, ok := c.allHandlerFunc[msghead.Cmd]; ok {

		req := &GRequest{Connect: &conn, Head: msghead, DataLen: msghead.BodyLen, DataBuffer: body}
		rsp := &GResponse{Connect: &conn}
		handler(req, rsp)
	} else {
		log.Warn("Never register processing method [%v]", msghead.Cmd)
	}
}

// RequestSpecifiedNode 向指定节点发送数据
func (c *CoreServer) SendPackage(data []byte) error {
	if len(data) <= 0 {
		return errors.New("data is empty")
	}

	_, err := c.connect.Write(data)

	return err
}

/////////////////////////////////////////////
///////  Server  ////////////////////////////
/////////////////////////////////////////////

func (c *CoreServer) InitConnectAsServer(port string) (err error) {

	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:"+port)
	if err != nil {
		log.Fatal(err.Error())
		return err
	}
	listen, err := net.ListenTCP("tcp", addr)
	defer listen.Close()
	if err != nil {
		log.Fatal(err.Error())
		return err
	}
	c.ListenPort = port

	log.Info("服务器正常启动,开始监听%v端口", port)

	// 监听
	go func(listen *net.TCPListener) {
		for {
			conn, err := listen.Accept()
			if err != nil {
				log.Fatal(err.Error())
			}
			go c.handleServerConn(conn)
		}
	}(listen)

	complete := make(chan int, 1)
	<-complete
	return nil
}

func (c *CoreServer) handleServerConn(conn net.Conn) {
	log.Debug("===>>> New Connection ===>>>")

	head := new(MessageHead)
	unhandledData := make([]byte, 0)

DISCONNECT:
	for {
		buf := make([]byte, BufLength)
		for {
			n, err := conn.Read(buf)
			if err != nil && err != io.EOF {
				log.Warn("读取缓冲区出错，有可能是连接断开了: %v", err.Error())
				c.deliverMessage(conn, &MessageHead{Cmd: 3, Version: 0, HeadLen: 6, BodyLen: 0}, nil)
				break DISCONNECT
			}

			unhandledData = append(unhandledData, buf[:n]...)

			if n != BufLength {
				break
			}
		}

		if len(unhandledData) == 0 {
			log.Warn("读取到的数据长度为0，有可能是连接断开了")
			c.deliverMessage(conn, &MessageHead{Cmd: 3, Version: 0, HeadLen: 6, BodyLen: 0}, nil)
			break
		}
		// log.Debug("接收到数据：%v", unhandledData)

		for nil == head.Unpack(unhandledData) {
			// log.Debug("解析出消息：%v", unhandledData)

			msgLen := head.BodyLen + uint16(head.HeadLen)
			if msgLen > uint16(len(unhandledData)) {
				log.Error("接收消息出现问题：长度= %v，缓冲=%v", msgLen, unhandledData)
				break
			}
			msgData := unhandledData[:msgLen]
			unhandledData = unhandledData[msgLen:]

			// BufferChan <- msgData
			if head.Cmd == 1 {
				// log.Debug("收到一个来自客户端的心跳, %s", conn.RemoteAddr().String())
				m := MessageHead{Cmd: 1, Version: 0, HeadLen: 6, BodyLen: 0}
				conn.Write(m.Pack())
			} else {
				// log.Debug("收到一个来自客户端的业务包, %d", head.Cmd)
				c.deliverMessage(conn, head, msgData[head.HeadLen:])
			}
		}
	}
	log.Debug("===>>> Connection closed ===>>>")
}

/////////////////////////////////////////////
///////  Client  ////////////////////////////
/////////////////////////////////////////////

// InitConnectAsClient  a
func (c *CoreServer) InitConnectAsClient(masterAddr string, complete chan int) (err error) {

	addr, err := net.ResolveTCPAddr("tcp", masterAddr)
	if nil != err {
		log.Error("ResolveTCPAddr %s error:", masterAddr)
		return err
	}
	conn, err := net.DialTCP("tcp", nil, addr)
	if nil != err {
		log.Error("DialTCP %s error:", masterAddr)
		return err
	}

	go c.handleClientConn(conn, complete)

	c.remoteAddr = masterAddr
	c.connect = conn

	clientHeartBeating(conn)
	return nil
}

func (c *CoreServer) handleClientConn(conn net.Conn, complete chan int) {
	log.Debug("===>>> New Connection ===>>>")

	head := new(MessageHead)
	unhandledData := make([]byte, 0)

DISCONNECT:
	for {
		buf := make([]byte, BufLength)
		for {
			n, err := conn.Read(buf)
			if err != nil && err != io.EOF {
				log.Warn("读取缓冲区出错，有可能是连接断开了: %v", err.Error())
				conn.Close()
				complete <- 2
				break DISCONNECT
			}

			unhandledData = append(unhandledData, buf[:n]...)

			if n != BufLength {
				break
			}
		}

		if len(unhandledData) == 0 {
			log.Warn("读取到的数据长度为0，有可能是连接断开了")
			conn.Close()
			complete <- 1
			break
		}
		// log.Debug("接收到数据：%v", unhandledData)

		for nil == head.Unpack(unhandledData) {
			msgLen := head.BodyLen + uint16(head.HeadLen)
			msgData := unhandledData[:msgLen]
			unhandledData = unhandledData[msgLen:]

			log.Debug("收到服务器信息：%+v", head)
			clientHeartBeating(conn)

			if head.Cmd != 1 {
				c.deliverMessage(conn, head, msgData[head.HeadLen:])
			}
		}
	}
	log.Debug("===>>> Connection closed ===>>>")
}

var clientHeartTimer *time.Timer

func clientHeartBeating(conn net.Conn) {

	if clientHeartTimer != nil {
		clientHeartTimer.Stop()
	}

	clientHeartTimer = time.AfterFunc(time.Second*10, func() {
		m := MessageHead{Cmd: 1, Version: 0, HeadLen: 6, BodyLen: 0}
		conn.Write(m.Pack())
	})

}
