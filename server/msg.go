package server

import (
	"encoding/json"
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
type RoomConnChange struct {
	Room string
	Conn socketio.Conn
}
//用于客户端返回确认接收消息后同步数据
type MsgConfirm struct {
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
			commonMsgChan:      make(chan *PostMsg,1024),
			ensureMsgChan:      make(chan *PostMsg,100),
			JoinRoomChan:       make(chan *RoomConnChange,256),
			LeavaRoomChan:      make(chan *RoomConnChange,256),
			LeaveAllRoomChan:	make(chan socketio.Conn,100),
			MsgConnConfirmChan: make(chan *MsgConfirm,256),
			EnsureMsgSendMap:   make(map[string]*EnsureMsgSendInfo),
			roomConns:          make(map[string]map[string]socketio.Conn),
			roomConnLock:       sync.RWMutex{},
			msgSendLock:        sync.RWMutex{},
		}
	})
	return msgManager
}

type MsgManager struct {
	commonMsgChan      chan *PostMsg                       //普通消息通道
	ensureMsgChan      chan *PostMsg                       //确保送达消息的通道
	JoinRoomChan       chan *RoomConnChange                //离开房间的通信通道
	LeavaRoomChan      chan *RoomConnChange                //进入房间的通信通道
	LeaveAllRoomChan   chan socketio.Conn				   //用于客户端异常断开等情况退出所有房间
	MsgConnConfirmChan chan *MsgConfirm                    //客户端消息确认同步通道
	EnsureMsgSendMap   map[string]*EnsureMsgSendInfo       //存放消息的id和客户端id的映射,以及该发送给该客户端发送消息的时间
	roomConns          map[string]map[string]socketio.Conn //第一个key是房间号，第二个key是链接id
	roomConnLock       sync.RWMutex                        //控制房间和链接的读写锁
	msgSendLock        sync.RWMutex                        //用于控制确认送达消息读写的锁
}

func (s *MsgManager) Run(){
	go s.loopRoomConns()
	go s.loopCommonMsg()
	go s.loopEnsureMsg()
	go s.ensureMsg()
}

func (s *MsgManager)loopCommonMsg(){
	wsServer := GetWsServer()
	for  {
		msg := <-s.commonMsgChan
		jsonMsg,err := json.Marshal(msg)
		if err != nil{
			log.Println("Marshal common msg error with msg data :"+msg.PostData.(string))
			continue
		}
		log.Println("send common message")
		wsServer.BroadcastToRoom("/",msg.Room,"commonMessage",string(jsonMsg))
	}
}

func (s *MsgManager) loopEnsureMsg(){
	wsServer := GetWsServer()
	for  {
		msg := <-s.ensureMsgChan
		emsi := EnsureMsgSendInfo{
			msg:       msg,
			startTime: time.Now(),
			sendIdMap: make(map[string]socketio.Conn),
		}
		s.msgSendLock.Lock()
		s.EnsureMsgSendMap[msg.Id] = &emsi
		s.msgSendLock.Unlock()
		log.Println("send ensure message")
		emsi.Lock()
		wsServer.ForEach("/",msg.Room, func(conn socketio.Conn) {
			//将conn放入发送map中
			emsi.sendIdMap[conn.ID()] = conn
			s.sendEnsureMsg(conn,msg)
		})
		emsi.Unlock()
	}
}

func (s *MsgManager) sendEnsureMsg(conn socketio.Conn,msg *PostMsg){
	jsonMsg,err := json.Marshal(msg)
	if err != nil{
		log.Println("Marshal ensure msg error with msg data :"+msg.PostData.(string))
		return
	}
	conn.Emit("ensureMessage",string(jsonMsg))
}

func (s *MsgManager) loopRoomConns(){
	for {
		select {
			case join := <- s.JoinRoomChan:
				log.Println(join.Conn.ID()+" join room "+join.Room)
				s.roomConnLock.Lock()
				if idConnMap,ok := s.roomConns[join.Room];ok{
					idConnMap[join.Conn.ID()] = join.Conn
				}else{
					s.roomConns[join.Room] = map[string]socketio.Conn{join.Conn.ID():join.Conn}
				}
				//输出个个房间中客户端的数量
				for room,roomMap := range s.roomConns{
					log.Println("room "+room+" has client num "+strconv.Itoa(len(roomMap)))
				}
				s.roomConnLock.Unlock()
			case leave := <- s.LeavaRoomChan:
				log.Println(leave.Conn.ID()+" leave room "+leave.Room)
				s.roomConnLock.Lock()
				if m,ok := s.roomConns[leave.Room];!ok{
					break
				}else{
					delete(m,leave.Conn.ID())
				}
				s.roomConnLock.Unlock()
			case conn := <- s.LeaveAllRoomChan:
				log.Println(conn.ID()+" leave all room ")
				s.roomConnLock.Lock()
				for _,room := range s.roomConns{
					delete(room,conn.ID())
				}
				s.roomConnLock.Unlock()
		}
	}
}
//检查某一个链接是否在某一个房间内
func (s *MsgManager) RoomHasConn(room string, conn socketio.Conn) bool{
	s.roomConnLock.RLock()
	defer s.roomConnLock.RUnlock()
	if roomMap,ok := s.roomConns[room];!ok{
		return false
	}else{
		if _,ok := roomMap[conn.ID()];!ok{
			return false
		}else{
			return true
		}
	}
}
//消息发送后确保发送成功
func (s *MsgManager) ensureMsg(){
	go func() {//同步确认接收情况
		for{
			mcc := <- s.MsgConnConfirmChan
			log.Println(mcc.Conn.ID() + " confirm msg "+mcc.MsgId)
			s.msgSendLock.Lock()
			if mmap,ok := s.EnsureMsgSendMap[mcc.MsgId];ok{
				mmap.Lock()
				delete(mmap.sendIdMap,mcc.Conn.ID())
				mmap.Unlock()
			}
			s.msgSendLock.Unlock()
		}
	}()
	for {
		s.msgSendLock.RLock()
		for _,info := range s.EnsureMsgSendMap{
			if len(info.sendIdMap) < 0{
				continue
			}
			//超过30s,且客户端为离开房间则重新发送消息
			if time.Now().Unix() > info.startTime.Add(time.Second*20).Unix(){
				info.startTime = time.Now()
				for _,v := range info.sendIdMap{
					if s.RoomHasConn(info.msg.Room,v){
						s.sendEnsureMsg(v,info.msg)
					}else{
						info.Lock()
						delete(info.sendIdMap,v.ID())
						info.Unlock()
					}
				}
			}
		}
		s.msgSendLock.RUnlock()
		time.Sleep(time.Second*2)
	}
}

func (s *MsgManager)DispatchMsg(room,postData,isEnsure string){
	msg := newPostMsg(room,postData,isEnsure)
	if(msg.IsEnsure){
		s.ensureMsgChan <- msg
	}else{
		s.commonMsgChan <- msg
	}
}

func (s *MsgManager)JoinRoom(room string,conn socketio.Conn){
	roomChange := &RoomConnChange{
		room,
		conn,
	}
	s.JoinRoomChan <- roomChange
}

func (s *MsgManager)LeaveRoom(room string,conn socketio.Conn){
	roomChange := &RoomConnChange{
		room,
		conn,
	}
	s.LeavaRoomChan <- roomChange
}

func (s *MsgManager)LeaveAllRoom(conn socketio.Conn){
	s.LeaveAllRoomChan <- conn
}

func (s *MsgManager)ConfirmMsg(conn socketio.Conn,msgId string){
	msgConf := MsgConfirm{
		msgId,
		conn,
	}
	s.MsgConnConfirmChan <- &msgConf
}



