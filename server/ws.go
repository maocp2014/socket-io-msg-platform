package server

import (
	"github.com/googollee/go-socket.io"
	"log"
	"strconv"
	"sync"
)

var wsServer *socketio.Server

func GetWsServer()*socketio.Server{
	if wsServer != nil{
		return wsServer
	}
	once := sync.Once{}
	once.Do(func() {
		newServer, err := socketio.NewServer(nil)
		if err != nil {
			log.Fatal(err)
		}
		wsServer = newServer
		RegisterEnvet()
	})
	return wsServer
}

func RegisterEnvet(){
	server := GetWsServer()
	server.OnConnect("/", func(conn socketio.Conn) error {
		log.Println(conn.ID()+" "+conn.RemoteAddr().String()+" connect ")
		return nil
	})

	server.OnError("/", func(conn socketio.Conn, e error) {
		//on error 时不能调用任何conn相关的方法，因为此时无法保证底层实现conn接口的对象是否已经被释放引发panic
		log.Println("meet error "+e.Error())
	})

	//加入房间事件
	server.OnEvent("/", "joinRoom", func(conn socketio.Conn, room string) {
		log.Println(conn.ID()+" "+conn.RemoteAddr().String()+" joinRoom "+room)
		GetMsgManager().JoinRoom(room,conn)
		conn.Join(room)
	})

	//离开房间
	server.OnEvent("/", "leaveRoom", func(conn socketio.Conn, room string) {
		log.Println(conn.ID()+" "+conn.RemoteAddr().String()+" leaveRoom "+room)
		GetMsgManager().LeaveRoom(room,conn)
		conn.Leave(room)
	})

	//离开所有房间事件
	server.OnEvent("/", "leaveAllRoom", func(conn socketio.Conn) {
		log.Println(conn.ID()+" "+conn.RemoteAddr().String()+" leaveAllRoom")
		GetMsgManager().LeaveAllRoom(conn)
		conn.LeaveAll()
	})

	//确认消息事件
	server.OnEvent("/", "confirmMessage", func(conn socketio.Conn,msgId uint64){
		log.Println(conn.ID()+" "+conn.RemoteAddr().String()+" confirmMessage "+strconv.FormatUint(msgId,10))
		GetMsgManager().ConfirmMsg(conn,msgId)
	})
	
	server.OnDisconnect("/", func(conn socketio.Conn, s string) {
		log.Println(conn.ID()+" "+conn.RemoteAddr().String()+" disconnect "+s)
		GetMsgManager().LeaveAllRoom(conn)
		conn.LeaveAll()
	})
}