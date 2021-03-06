//房间

package bubble

import (
	"fmt"
	"math"
	"time"
)

const (
	ROOM_MAP_WIDTH  = 18 //格子宽
	ROOM_MAP_HEIGHT = 10 //格子高.

	ROOM_TILED_WIDTH  = 64 //格子宽
	ROOM_TILED_HEIGHT = 64 //格子宽
)

//
type Point struct {
	x int32
	y int32
}

//一个炸弹房间
type Room struct {
	bubbleId   int32
	mm         [ROOM_MAP_HEIGHT][ROOM_MAP_WIDTH]int
	bubbleList []Bubble
	mq         chan Msg
	u1_id      int32
	u2_id      int32
	u1         chan Msg //玩家1
	u2         chan Msg //玩家2
	//实时维护两个玩家的位置tiled，可用于穿墙检测和爆炸检查.
	u1_pos Point
	u2_pos Point

	u1_stuck int32    //stuck时间.
	u2_stuck int32    //

	u1_status int32    //status时间.
	u2_status int32    //
}

// 新建一个房间
func MakeRoom(u1 chan Msg, u2 chan Msg) *Room {
	mq := make(chan Msg)
	fmt.Printf("u1 %v u2 %v\n", u1, u2)
	room := Room{u1: u1, u2: u2, mq: mq}
	room.mm = [ROOM_MAP_HEIGHT][ROOM_MAP_WIDTH]int{
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3, 0},
		{0, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0},
		{0, 0, 1, 3, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3, 1, 0, 0},
		{0, 0, 1, 3, 3, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 1, 0, 0},
		{0, 0, 1, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 1, 0, 0},
		{0, 0, 1, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1, 3, 3, 1, 0, 0},
		{0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3, 3, 1, 0, 0},
		{0, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0},
		{0, 0, 0, 0, 0, 3, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	}
	room.bubbleList = make([]Bubble, 0)
	room.u1_pos = Point{1, 1}
	room.u2_pos = Point{ROOM_MAP_WIDTH - 2, 1}
	return &room
}

//房间建立时通知各个玩家.
func (r *Room) NotifyReady() {
	u1d := RoomReadMsg{id: 0, roomMq: r.mq}
	u2d := RoomReadMsg{id: 1, roomMq: r.mq}
	r.u1 <- Msg{t: MSG_T_ROOM_READY, d: u1d}
	r.u2 <- Msg{t: MSG_T_ROOM_READY, d: u2d}
	r.u1_id = 0
	r.u2_id = 1
}

// 这时候玩家channel有可能已经被关闭，需要捕获异常。
func (r *Room) notifyClose(mq chan Msg) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Printf("Notify close Panice %v\n", mq)
		}
	}()
	msg := Msg{t: MSG_T_ROOM_CLOSE}
	mq <- msg
}

// 房间关闭通知玩家。
func (r *Room) NotifyClose() {
	r.notifyClose(r.u1)
	r.notifyClose(r.u2)
}

//关闭房间
func (r *Room) Stop() {
	close(r.mq)
}

// 有玩家退出，告诉其他玩家，并自己退出
func (r *Room) MsgQuit(msg Msg) int {
	r.NotifyClose()
	r.Stop()
	return -1
}

//位置改变,实时同步玩家位置.
func (r *Room) MsgTbRoomUseChg(msg RoomMsg) int {
	_, uchg, _ := BzReadRoomUserChg(msg.d)
	pos := uchg.user.pos
	x, y := r.tileCoordForPosition(pos.x, pos.y)
	if uchg.uIdx == r.u1_id {
		r.u1_pos = Point{x, y}
	} else {
		r.u2_pos = Point{x, y}
	}
	fmt.Printf("u %d x %d y %d\n", uchg.uIdx, x, y)

	r.CheckDie()
	return 0
}

// 转发消息给同一房间的其他玩家.
func (r *Room) MsgTcpBinDispatch(msg Msg) int {
	rMsg := msg.d.(RoomMsg)
	switch {
	case rMsg.t == BZ_ROOMUSERCHG:
		return r.MsgTbRoomUseChg(rMsg)
	}
	return 0
}

// 转发消息给同一房间的其他玩家.
func (r *Room) MsgTcpBin(msg Msg) int {
	castMsg := msg.d.(RoomMsg)
	if int32(castMsg.uid) == r.u1_id {
		r.u2 <- msg
	} else {
		r.u1 <- msg
	}
	//分发处理，如果你感兴趣.
	r.MsgTcpBinDispatch(msg)
	return 0
}

//根据坐标计算格子|这个函数有问题
func (r *Room) positionForTileCoord(x1 int32, y1 int32) (x2 int32, y2 int32) {
	x2 = (x1-1)*+ROOM_TILED_WIDTH + ROOM_TILED_WIDTH/2
	y2 = (y1-1)*+ROOM_TILED_HEIGHT + ROOM_TILED_HEIGHT/2
	return
}

//根据坐标计算格子.
func (r *Room) tileCoordForPosition(x1 int32, y1 int32) (x2 int32, y2 int32) {

	x2 = x1 / ROOM_TILED_WIDTH
	y2 = (ROOM_MAP_HEIGHT*ROOM_TILED_HEIGHT - y1) / ROOM_TILED_HEIGHT

	y2 = int32(math.Min(math.Max(0, float64(y2)), ROOM_MAP_HEIGHT-1))
	x2 = int32(math.Min(math.Max(0, float64(x2)), ROOM_MAP_WIDTH-1))

	return
}

//处理客户端逻辑消息，并转发给别人.
func (r *Room) MsgSetBubble(msg Msg) int {
	castMsg := msg.d.(RoomMsg)
	_, setBubble, _ := BzReadSetBubble(castMsg.d)
	r.bubbleId += 1
	setBubble.b.id = r.bubbleId

	now := int32(time.Now().UnixNano() / 1000000000)
	setBubble.b.keeptime += now

	tx, ty := r.tileCoordForPosition(setBubble.b.pos.x, setBubble.b.pos.y)
	fmt.Printf("MsgSetBubble %d x %d y %d \n", setBubble.b.id,
		setBubble.b.pos.x, setBubble.b.pos.y)
	fmt.Printf("MsgSetBubble  tx %d ty %d \n", tx, ty)

	r.bubbleList = append(r.bubbleList, *setBubble.b)

	dbyte, _ := BzWriteSetBubble(make([]byte, 0), setBubble)
	castMsg.d = dbyte

	msg.d = castMsg
	//修改格式可让接收者转发给客户端。
	msg.t = MSG_T_TCP_BIN
	r.u2 <- msg
	r.u1 <- msg

	return 0
}

// 消息分发
func (r *Room) MsgRoom(msg Msg) int {
	fmt.Printf("MsgRoom dd %s\n", msg)
	castMsg := msg.d.(RoomMsg)
	switch {
	case castMsg.t == BZ_SETBUBBLEREQ:
		return r.MsgSetBubble(msg)
	}
	return 0
}

// 房间消息，各种逻辑

// 分发消息
func (r *Room) dispatchMsg(msg Msg) int {
	switch {
	case msg.t == MSG_T_QUIT:
		return r.MsgQuit(msg)
	case msg.t == MSG_T_TCP_BIN:
		return r.MsgTcpBin(msg)
	case msg.t == MSG_T_ROOM_MSG:
		return r.MsgRoom(msg)
	}
	return 0
}

// 泡泡爆炸逻辑，计算更新地图
// 把格子可以爆的爆掉。
// 有些玩家被爆掉的，将其stuck.
func (r *Room) BubbleBombConflict(b Bubble) (destroyTiles []*BVector2,
	destroyUsers []int32) {
	//泡泡所在的格子，计算泡泡可以摧毁的格子.
	//tx, ty := r.tileCoordForPosition(b.pos.x, b.pos.y)
	tx, ty := b.pos.x, b.pos.y
	power := b.power + 2

	//x坐标的攻击范围
	tx_min := int32(math.Max(0, float64(tx-power)))
	tx_max := int32(math.Min(float64(ROOM_MAP_WIDTH-1), float64(tx+power)))
	//fmt.Printf("conflit tx_min %d tx_max %d\n", tx_min, tx_max)

	//y坐标的攻击范围
	ty_min := int32(math.Max(0, float64(ty-power)))
	ty_max := int32(math.Min(float64(ROOM_MAP_HEIGHT-1), float64(ty+power)))
	//fmt.Printf("conflit ty_min %d ty_max %d\n", ty_min, ty_max)

	//计算x轴上被摧毁的格子
	i := tx_min
	for i < tx_max {
		//fmt.Printf("tx %d ty %d %d\n", i, ty, r.mm[ty][i])
		switch v := r.mm[ty][i]; {
		case v == 1:
			r.mm[ty][i] = 1
			v := BVector2{x: i, y: ty}
			destroyTiles = append(destroyTiles, &v)
		}
		//爆到玩家
		if r.u1_pos.x == i && r.u1_pos.y == ty {
			r.SetUserStatus(r.u1_id, 5)
			//destroyUsers = append(destroyUsers, r.u1_id)
		}

		if r.u2_pos.x == i && r.u2_pos.y == ty {
			r.SetUserStatus(r.u2_id, 5)
			//destroyUsers = append(destroyUsers, r.u2_id)
		}
		//
		i++
	}

	//计算y轴上被摧毁的格子
	j := ty_min
	for j < ty_max {
		//fmt.Printf("tx %d ty %d %d\n", tx, j, r.mm[j][tx])
		switch v := r.mm[j][tx]; {
		case v == 1:
			r.mm[j][tx] = 1
			v := BVector2{x: tx, y: j}
			destroyTiles = append(destroyTiles, &v)
		}
		//爆到玩家
		if r.u1_pos.x == tx && r.u1_pos.y == j {
			r.SetUserStatus(r.u1_id, 5)
			//destroyUsers = append(destroyUsers, r.u1_id)
		}

		if r.u2_pos.x == tx && r.u2_pos.y == j {
			r.SetUserStatus(r.u2_id, 5)
			//destroyUsers = append(destroyUsers, r.u2_id)
		}
		//
		j++
	}

	return
}

// stuck状态的玩家相交则die.
func (r *Room) CheckDie() {
	if(r.u1_pos != r.u2_pos){
		return
	}

	if(r.u1_status==5){
		r.u1_status = 6
		r.NotifyUserStatus(r.u1_id,6)
	}

	if(r.u2_status==5){
		r.u2_status = 6
		r.NotifyUserStatus(r.u2_id,6)
	}
}


// 设置玩家的状态。
func (r *Room) SetUserStatus(u int32, status int32) {
	now := int32(time.Now().UnixNano() / 1000000000)

	switch {
	case status == 5:
		//如果stuck，设置10s.
		r.NotifyUserStatus(u,status)
		if(u==r.u1_id) {
			r.u1_stuck = now + 5;
		}else{
			r.u2_stuck = now + 5;
		}
	}

	//设置状态
	if(u==r.u1_id){
		r.u1_status = status
	}else{
		r.u2_status = status
	}

}

// 通知客户端玩家状态变化.
func (r *Room) NotifyUserStatus(u int32, status int32) {
	roomUseChg := RoomUserStatusChg{id:u, status: status}
	bytes, _ := BzWriteRoomUserStatusChg(make([]byte, 0), &roomUseChg)

	var msg Msg
	msg.t = MSG_T_TCP_BIN
	msg.d = RoomMsg{t: 10, uid: 0, d: bytes}

	fmt.Printf("********************\n")
	fmt.Printf("Notify use chg %v\n", roomUseChg)

	r.u2 <- msg
	r.u1 <- msg
}

// 泡泡爆炸逻辑，通知
func (r *Room) BubbleBomb(b Bubble) {
	//通知客户端泡泡爆炸，并且有些玩家和格子被摧毁了。
	var bubbleBomb BubbleBomb
	bubbleBomb.id = b.id
	//玩家被泡泡住会有10s的stuck时间.
	bubbleBomb.destroyTiles, bubbleBomb.destroyUsers = r.BubbleBombConflict(b)

	fmt.Printf("destory tile %v\n", bubbleBomb.destroyTiles)

	bytes, _ := BzWriteBubbleBomb(make([]byte, 0), &bubbleBomb)
	var msg Msg
	msg.t = MSG_T_TCP_BIN
	msg.d = RoomMsg{t: 9, uid: 0, d: bytes}

	r.u2 <- msg
	r.u1 <- msg
}

//检查玩家状态，stuck10s被内撞则死.
func (r *Room) UsersCheck() {
	now := int32(time.Now().UnixNano() / 1000000000)
	if(r.u1_status == 5 && r.u1_stuck < now){
		r.u1_status = 0
		r.NotifyUserStatus(r.u1_id, 0)
	}

	if(r.u2_status == 5 && r.u2_stuck < now){
		r.u2_status = 0
		r.NotifyUserStatus(r.u2_id, 0)
	}
}

//检查泡泡，设置爆炸.
func (r *Room) BubbleCheck() {
	var length int
	var bubble Bubble
	now := int32(time.Now().UnixNano() / 1000000000)

	for {
		length = len(r.bubbleList)
		if length == 0 {
			return
		}
		bubble = r.bubbleList[0]
		fmt.Printf("bubble:%d keeptime:%d now:%d\n", bubble.id, bubble.keeptime, now)

		if bubble.keeptime <= now {
			r.BubbleBomb(bubble)
		} else {
			//直接退出，没有更多可以爆的呢。
			return
		}
		r.bubbleList = r.bubbleList[1:]
	}
}

// 需要设置心跳，如果超出时间没有任何消息，主动退出。
func (r *Room) Run() {
	var msg Msg
	var status bool
	bubbleChkTimer := time.After(time.Second)

	for {
		select {
		case msg, status = <-r.mq:
			fmt.Printf("room mq %v %v\n", msg, status)
			if status == false {
				return
			}
			//返回0时退出.
			if r.dispatchMsg(msg) < 0 {
				return
			}
		case <-bubbleChkTimer:
			//定时检查泡泡爆炸
			r.BubbleCheck()
			r.UsersCheck()
			bubbleChkTimer = time.After(time.Second)
		}

	}
}
