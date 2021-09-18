package main

import (
	"encoding/json"
	"fmt"
	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/sirupsen/logrus"
	"net/http"
	"os"
	"runtime"
	"socket-io-msg-platform/config"
	"socket-io-msg-platform/server"
	"time"
)

type WebResult struct {
	Code int				`json:"code"`
	Msg string				`json:"msg"`
	ServerTime  int64		`json:"serverTime"`
	Data interface{}		`json:"data"`
}

func init(){
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func receivePost(w http.ResponseWriter,r *http.Request){
	room := r.PostFormValue("room")
	postData := r.PostFormValue("jsonData")
	isEnsure := r.PostFormValue("isEnsure")
	token := r.PostFormValue("token")
	if token != config.GetMsgToken(){
		result := WebResult{
			Code:400,
			Msg:"token error",
			ServerTime:time.Now().Unix(),
			Data:nil,
		}
		jsonResult,_ := json.Marshal(&result)
		w.Write(jsonResult)
		return
	}
	logrus.Info("receive post msg room------"+room)
	logrus.Info("receive post msg ------"+postData)
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

//初始化日志记录
func initLog(){
	filePath := config.GetLogFile()+string(os.PathSeparator)+"msg_platform.%Y%m%d%H%M"
	logf,err := rotatelogs.New(filePath)
	if err != nil{
		panic(err.Error())
	}
	logrus.SetOutput(logf)
	//logrus.SetReportCaller(true)
	logrus.SetFormatter(&logrus.JSONFormatter{})
}

func main() {
	//初始化配置
	config.InitConfig("./msgPlatform.toml")
	//初始化日志记录配置
	initLog()

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

	listenAddr := fmt.Sprintf(":%d",config.GetServerPort())
	logrus.Info("Serving at localhost"+listenAddr)
	logrus.Info(http.ListenAndServe(listenAddr, nil))
}
