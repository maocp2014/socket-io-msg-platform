package main

import (
	"bufio"
	"encoding/json"
	"github.com/zhouhui8915/go-socket.io-client"
	"log"
	"os"
)

type PostMsg struct {
	Id uint64					`json:"msgId"`			//消息id，用于ws客户端和服务器确认收到该消息
	Room string					`json:"room"`			//房间号
	PostData interface{}		`json:"postData"`		//接口收到的数据，原样转发给ws客户端
	IsEnsure bool				`json:"isEnsure"`		//是否需要保证成功送达
}

func main() {

	opts := &socketio_client.Options{
		//Transport:"polling",
		Transport:"websocket",
		Query:     make(map[string]string),
	}
	opts.Query["user"] = "user"
	opts.Query["pwd"] = "pass"
	uri := "http://127.0.0.1:8099"

	client, err := socketio_client.NewClient(uri, opts)
	if err != nil {
		log.Printf("NewClient error:%v\n", err)
		return
	}
	//once := sync.Once{}
	client.On("error", func() {
		log.Printf("on error\n")
	})
	client.On("connection", func() {
		log.Printf("on connect\n")
	})
	client.On("commonMessage", func(msg string) {
		log.Printf("on commonMessage:%v\n", msg)
	})
	client.On("ensureMessage", func(msg string) {
			log.Printf("on ensureMessage:%v\n", msg)
			postMsg := PostMsg{}
			json.Unmarshal([]byte(msg),&postMsg)
			client.Emit("confirmMessage",postMsg.Id)
	})
	client.On("disconnection", func() {
		log.Printf("on disconnect\n")
		os.Exit(0)
	})

	reader := bufio.NewReader(os.Stdin)
	for {
		data, _, _ := reader.ReadLine()
		command := string(data)
		switch command {
			case "join":
				client.Emit("joinRoom","live")
				log.Println("join room "+"live")
			case "leave":
				client.Emit("leaveRoom","live")
				log.Println("leave room "+"live")
			case "leaveAll":
				client.Emit("leaveAllRoom")
				log.Println("leaveAllRoom")
		}
	}
}