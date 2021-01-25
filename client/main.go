package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/zhouhui8915/go-socket.io-client"
	"log"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

type PostMsg struct {
	Id string					`json:"msgId"`			//消息id，用于ws客户端和服务器确认收到该消息
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

func stressTest(uri string,opts *socketio_client.Options,nums int){
	stopChan := make(chan int,1024)
	receiveMsgChan := make(chan int,1024)

	for i:=0;i< nums;i++  {
		go connnetServer(uri,opts,i,stopChan,receiveMsgChan)
		time.Sleep(time.Millisecond*50)
	}

	var i int32 = 0
	var j int32 = 0

	for {
		select {
		case val := <-stopChan:
			fmt.Println(strconv.Itoa(val) + " disconect")
			atomic.AddInt32(&i, 1)
			fmt.Println("disconnect num " + strconv.Itoa(int(i)))
		case index := <-receiveMsgChan:
			fmt.Println(strconv.Itoa(index) + " receive msg")
			atomic.AddInt32(&j, 1)
			fmt.Println("receive num " + strconv.Itoa(int(j)))
		}
	}
}

func connnetServer(uri string,opts *socketio_client.Options,i int,stopChan chan int,receiveChan chan int){
	client, err := socketio_client.NewClient(uri, opts)
	if err != nil {
		log.Printf("NewClient error:%v\n", err)
		return
	}
	client.On("error", func() {
		log.Printf("on error\n")
	})
	client.On("connection", func() {
		log.Printf("on connect\n")
	})
	client.On("commonMessage", func(msg string) {
		//log.Printf("on commonMessage:%v\n", msg)
		receiveChan <- i
	})
	client.On("ensureMessage", func(msg string) {
		log.Printf("on ensureMessage:%v\n", msg)
		postMsg := PostMsg{}
		json.Unmarshal([]byte(msg), &postMsg)
		client.Emit("confirmMessage", postMsg.Id)
	})
	client.On("disconnection", func() {
		stopChan <- i
		//log.Printf("on disconnect\n")
		//os.Exit(0)
	})
}