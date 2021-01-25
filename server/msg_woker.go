package server

import (
	"encoding/json"
	"github.com/googollee/go-socket.io"
	"log"
	"sync"
	"time"
)
//发送消息的worker
type MsgWorker struct {
	id int
	msgChan chan *PostMsg								//用于接收待发送的消息
	msgChanPool chan chan *PostMsg
	disPatcher *MsgDispatcher
}

//即来即走，完成工作报备
func (w *MsgWorker)work(){
	for {
		//每次空闲时，woker将自己接受消息的通道注册到工作池
		w.msgChanPool <- w.msgChan
		select {
			case msg := <- w.msgChan:
				//需要确认的消息
				if msg.IsEnsure {
					w.disPatcher.sendEnsureMsg(msg)
				}else{
					w.disPatcher.sendCommonMsg(msg)
				}
		}
	}
}
//接收客户端消息回执的worker
type ConfirmMsgWorker struct {
	id                 int
	confirmMsgChan     chan *ConfirmMsg						//用于接收ws客户端发送的确认消息回执
	ConfirmMsgChanPool chan chan *ConfirmMsg
	disPatcher         *MsgDispatcher
}

//即来即走，完成工作报备
func (w *ConfirmMsgWorker)work(){
	for {
		//每次空闲时，woker将自己接受消息的通道注册到工作池
		w.ConfirmMsgChanPool <- w.confirmMsgChan
		select {
			case msg := <- w.confirmMsgChan:
				w.disPatcher.doConfirmMsg(msg)
		}
	}
}

type MsgDispatcher struct {
	msgChan     chan *PostMsg      					//带缓冲的通道，用于接收外部传进来的msg
	msgChanPool chan chan *PostMsg 					//接收准备好的worker的msg通道
	msgWorkers  []*MsgWorker       					//worker数组，用于控制woker

	ConfirmMsgChan   chan *ConfirmMsg              //客户端消息确认同步通道
	ConfirmMsgChanPool chan chan *ConfirmMsg
	confirmWorkers []*ConfirmMsgWorker

	EnsureMsgSendMap map[string]*EnsureMsgSendInfo //存放消息的id和客户端id的映射,以及该发送给该客户端发送消息的时间
	msgSendLock      sync.RWMutex                  //用于控制确认送达消息读写的锁
}

func (p *MsgDispatcher)run(){
	//启动msg worker
	for _,worker := range p.msgWorkers {
		go worker.work()
	}
	//启动confirm worker
	for _,confirmWorker := range p.confirmWorkers{
		go confirmWorker.work()
	}

	//启动wokerPool
	go p.loopMsg()
	go p.ensureMsg()
}
//接收外部传进来的消息
func (p *MsgDispatcher)dispatchMsg(msg *PostMsg){
	p.msgChan <- msg
}

func (p *MsgDispatcher)confirmMsg(msg *ConfirmMsg){
	p.ConfirmMsgChan <- msg
}

func (p *MsgDispatcher)loopMsg(){
	for{
		select {
			//从消息通道里取消息分给准备好的woker
			case msg := <- p.msgChan:
				workerChan := <- p.msgChanPool
				workerChan <- msg
			case confirmMsg := <- p.ConfirmMsgChan:
				confirmWorker := <- p.ConfirmMsgChanPool
				confirmWorker <- confirmMsg
		}
	}
}

//发送普通消息
func (p *MsgDispatcher)sendCommonMsg(msg *PostMsg){
	jsonMsg,err := json.Marshal(msg)
	if err != nil{
		log.Println("Marshal msg error with msg data :"+msg.PostData.(string))
		return
	}
	log.Println("send common message")
	wsServer.BroadcastToRoom("/",msg.Room,"commonMessage",string(jsonMsg))
}

//发送需要确认的消息
func (p *MsgDispatcher)sendEnsureMsg(msg *PostMsg){
	emsi := EnsureMsgSendInfo{
		msg:       msg,
		startTime: time.Now(),
		sendIdMap: make(map[string]socketio.Conn),
	}
	p.msgSendLock.Lock()
	p.EnsureMsgSendMap[msg.Id] = &emsi
	p.msgSendLock.Unlock()
	log.Println("send ensure message")
	emsi.Lock()
	jsonMsg,err := json.Marshal(msg)
	if err != nil{
		log.Println("Marshal msg error with msg data :"+msg.PostData.(string))
		return
	}
	wsServer.ForEach("/",msg.Room, func(conn socketio.Conn) {
		//将conn放入发送map中
		emsi.sendIdMap[conn.ID()] = conn
		conn.Emit("ensureMessage",string(jsonMsg))
	})
	emsi.Unlock()
}
//处理客户端消息确认回执
func (p *MsgDispatcher) doConfirmMsg(msg *ConfirmMsg) {
	log.Println(msg.Conn.ID() + " confirm msg "+msg.MsgId)
	p.msgSendLock.Lock()
	if mmap,ok := p.EnsureMsgSendMap[msg.MsgId];ok{
		mmap.Lock()
		delete(mmap.sendIdMap,msg.Conn.ID())
		mmap.Unlock()
	}
	p.msgSendLock.Unlock()
}

//消息发送后确保发送成功
func (p *MsgDispatcher) ensureMsg(){
	for {
		msgManager := GetMsgManager()
		p.msgSendLock.RLock()
		for _,info := range p.EnsureMsgSendMap{
			if len(info.sendIdMap) < 0{
				continue
			}
			//超过30s,且客户端为离开房间则重新发送消息
			if time.Now().Unix() > info.startTime.Add(time.Second*20).Unix(){
				info.startTime = time.Now()
				for _,v := range info.sendIdMap{
					if msgManager.RoomHasConn(info.msg.Room,v){
						jsonMsg,err := json.Marshal(info.msg)
						if err != nil{
							log.Println("Marshal msg error with msg data :"+info.msg.PostData.(string))
							continue
						}
						v.Emit("ensureMessage",string(jsonMsg))
					}else{
						info.Lock()
						delete(info.sendIdMap,v.ID())
						info.Unlock()
					}
				}
			}
		}
		p.msgSendLock.RUnlock()
		time.Sleep(time.Second*2)
	}
}

//创建消息分发器，并初始化消息工作者
func NewMsgDispatcher(msgWorkNum,confirmMsgWorkerNum int)* MsgDispatcher{
	dispather := MsgDispatcher{
		msgChan:            make(chan *PostMsg,1024),
		msgChanPool:        make(chan chan *PostMsg, msgWorkNum),
		msgWorkers:         []*MsgWorker{},

		ConfirmMsgChan:     make(chan *ConfirmMsg,1024),
		ConfirmMsgChanPool: make(chan chan *ConfirmMsg, msgWorkNum),
		confirmWorkers:		[]*ConfirmMsgWorker{},

		msgSendLock:        sync.RWMutex{},
		EnsureMsgSendMap: make(map[string]*EnsureMsgSendInfo),
	}
	//初始化发送消息工作者
	for i:=0;i< msgWorkNum;i++{
		newWorker := MsgWorker{
			id:i,
			msgChan:make(chan *PostMsg,1),
			msgChanPool:dispather.msgChanPool,
			disPatcher:&dispather,
		}
		dispather.msgWorkers = append(dispather.msgWorkers,&newWorker)
	}
	//初始化确认消息回执工作者
	for i:=0;i< confirmMsgWorkerNum;i++{
		confirmWorker := ConfirmMsgWorker{
			id:                 i,
			confirmMsgChan:     make(chan *ConfirmMsg,1),
			ConfirmMsgChanPool: dispather.ConfirmMsgChanPool,
			disPatcher:         &dispather,
		}
		dispather.confirmWorkers = append(dispather.confirmWorkers,&confirmWorker)
	}

	return &dispather
}