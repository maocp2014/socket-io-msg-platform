package server

import (
	"github.com/googollee/go-socket.io"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var msgId uint32

type PostMsg struct {
	Id string					`json:"msgId"`			//消息id，用于ws客户端和服务器确认收到该消息
	Room string					`json:"room"`			//房间号
	PostData interface{}		`json:"postData"`		//接口收到的数据，原样转发给ws客户端
	IsEnsure bool				`json:"isEnsure"`		//是否需要保证成功送达
}

type EnsureMsgSendInfo struct {
	msg       *PostMsg
	startTime time.Time					   	//用于判定超时重发
	sendIdMap map[string]socketio.Conn      //key为链接的id，方便收到确认消息后删除链接
	sync.RWMutex
}
//用于离开和加入房间时通过通道同步数据
type ConnRoomChangeInfo struct {
	Room string
	Conn socketio.Conn
}
//用于客户端返回确认接收消息后同步数据
type ConfirmMsg struct {
	MsgId string
	Conn socketio.Conn
}

//产生全局唯一的消息id，考虑到服务可能会重启，所以需要加入时间因子
func getMsgId() string{
	msgId := atomic.AddUint32(&msgId,1)
	return strconv.FormatInt(time.Now().Unix(),10)+":"+strconv.FormatUint(uint64(msgId),10)
}

func newPostMsg(room string,postData string,isEnsure string)*PostMsg{
	msg := &PostMsg{
		Id:getMsgId(),
		Room:room,
		PostData:postData,
	}
	if isEnsure == "1"{
		msg.IsEnsure = true
	}else{
		msg.IsEnsure = false
	}

	return msg
}

var msgManager *MsgManager

func GetMsgManager() *MsgManager{
	if msgManager != nil{
		return msgManager
	}
	once := sync.Once{}
	once.Do(func() {
		msgManager = &MsgManager{
			msgDispatcher:    NewMsgDispatcher(10,5),
			JoinRoomChan:     make(chan *ConnRoomChangeInfo,256),
			LeavaRoomChan:    make(chan *ConnRoomChangeInfo,256),
			LeaveAllRoomChan: make(chan socketio.Conn,100),
			rooms:            sync.Map{},
		}
	})
	return msgManager
}

type MsgRoom struct {
	Name string
	ConnsMap sync.Map		//健名为conn.id,键值为conn
}

type MsgManager struct {
	msgDispatcher    *MsgDispatcher           //消息分发器
	JoinRoomChan     chan *ConnRoomChangeInfo //离开房间的通信通道
	LeavaRoomChan    chan *ConnRoomChangeInfo //进入房间的通信通道
	LeaveAllRoomChan chan socketio.Conn       //用于客户端异常断开等情况退出所有房间
	rooms            sync.Map                 //第一个key是房间号，值为msgRoom结构
}

func (s *MsgManager) Run(){
	go s.loopRoomConns()
	go s.msgDispatcher.run()
}
//用户进出房间的操作不会很频繁，暂时不用多协程处理
func (s *MsgManager) loopRoomConns(){
	for {
		select {
			case join := <- s.JoinRoomChan:
				s.doJoinRoom(join)
			case leave := <- s.LeavaRoomChan:
				s.doLeaveRoom(leave)
			case conn := <- s.LeaveAllRoomChan:
				s.doLeaveAllRoom(conn)
		}
	}
}
//客户端加入房间
func (s *MsgManager) doJoinRoom(join *ConnRoomChangeInfo){
	log.Println(join.Conn.ID()+" join room "+ join.Room)
	msgRoom,ok := s.rooms.Load(join.Room)
	if ok {
		msgRoom.(*MsgRoom).ConnsMap.Store(join.Conn.ID(), join.Conn)
	}else{
		newMsgRoom := MsgRoom{
			Name:     join.Room,
			ConnsMap: sync.Map{},
		}
		newMsgRoom.ConnsMap.Store(join.Conn.ID(), join.Conn)
		s.rooms.Store(join.Room,&newMsgRoom)
	}
}
//客户端离开房间
func (s *MsgManager) doLeaveRoom(leave *ConnRoomChangeInfo){
	log.Println(leave.Conn.ID()+" leave room "+leave.Room)
	msgRoom,ok := s.rooms.Load(leave.Room)
	if ok {
		msgRoom.(*MsgRoom).ConnsMap.Delete(leave.Conn.ID())
	}
}
//客户端离开所有房间
func (s *MsgManager) doLeaveAllRoom(conn socketio.Conn){
	log.Println(conn.ID()+" leave all room ")
	s.rooms.Range(func(k,v interface{}) bool{
		v.(*MsgRoom).ConnsMap.Delete(conn.ID())
		return true
	})
}


//检查某一个链接是否在某一个房间内
func (s *MsgManager) RoomHasConn(room string, conn socketio.Conn) bool{
	msgRoom,ok := s.rooms.Load(room)
	if ok {
		_,ok = msgRoom.(*MsgRoom).ConnsMap.Load(conn.ID())
		if ok {
			return true
		}
	}

	return false
}

func (s *MsgManager)DispatchMsg(room,postData,isEnsure string){
	msg := newPostMsg(room,postData,isEnsure)
	s.msgDispatcher.dispatchMsg(msg)
}

func (s *MsgManager)JoinRoom(room string,conn socketio.Conn){
	roomChange := &ConnRoomChangeInfo{
		room,
		conn,
	}
	s.JoinRoomChan <- roomChange
}

func (s *MsgManager)LeaveRoom(room string,conn socketio.Conn){
	roomChange := &ConnRoomChangeInfo{
		room,
		conn,
	}
	s.LeavaRoomChan <- roomChange
}

func (s *MsgManager)LeaveAllRoom(conn socketio.Conn){
	s.LeaveAllRoomChan <- conn
}

func (s *MsgManager)ConfirmMsg(conn socketio.Conn,msgId string){
	msgConf := ConfirmMsg{
		msgId,
		conn,
	}
	s.msgDispatcher.confirmMsg(&msgConf)
}



