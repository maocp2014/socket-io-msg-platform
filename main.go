package main

import (
	"encoding/json"
	"flag"
	"socket-io-msg-platform/server"
	"log"
	"net/http"
	"runtime"
	"time"
)

type WebResult struct {
	Code int				`json:"code"`
	Msg string				`json:"msg"`
	ServerTime  int64		`json:"serverTime"`
	Data interface{}		`json:"data"`
}

var listenAddr string

func init(){
	runtime.GOMAXPROCS(runtime.NumCPU())
	flag.StringVar(&listenAddr, "listen", ":8099", "--listen")
	flag.Parse()
}

func receivePost(w http.ResponseWriter,r *http.Request){
	room := r.PostFormValue("room")
	postData := r.PostFormValue("jsonData")
	isEnsure := r.PostFormValue("isEnsure")
	log.Println("receive post msg room------"+room)
	log.Println("receive post msg ------"+postData)
	server.GetMsgManager().DispatchMsg(room,postData,isEnsure)
	result := WebResult{
		Code:200,
		Msg:"success",
		ServerTime:time.Now().Unix(),
		Data:nil,
	}
	jsonResult,_ := json.Marshal(&result)
	w.Write(jsonResult)
}

func main() {
	wsServer := server.NewWsServer()
	server.GetMsgManager().Run()
	go wsServer.Serve()
	defer wsServer.Close()

	http.HandleFunc("/socket.io/", func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != ""{
			w.Header().Set("Access-Control-Allow-Origin",origin)
		}else{
			w.Header().Set("Access-Control-Allow-Origin","*")
		}
		w.Header().Set("Access-Control-Allow-Credentials","true")
		//删除Origin以避开websocket的域名校验
		r.Header.Del("Origin")
		wsServer.ServeHTTP(w,r)
	})
	http.HandleFunc("/postMsg",receivePost)
	log.Println("Serving at localhost"+listenAddr)
	log.Fatal(http.ListenAndServe(listenAddr, nil))
}
