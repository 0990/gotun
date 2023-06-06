package msg

const (
	TypeLogin         = 'o'
	TypeLoginResp     = '1'
	TypeNewWorkConn   = 'w'
	TypeStartWorkConn = 's'
	TypeReqWorkConn   = 'r'
	TypePing          = 'h'
	TypePong          = '4'

	TypeSocks5XReq  = 'q'
	TypeSocks5XResp = 'p'
)

var msgTypeMap = map[byte]interface{}{
	TypeLogin:         Login{},
	TypeLoginResp:     LoginResp{},
	TypeNewWorkConn:   NewWorkConn{},
	TypeReqWorkConn:   ReqWorkConn{},
	TypeStartWorkConn: StartWorkConn{},
	TypePing:          Ping{},
	TypePong:          Pong{},
	TypeSocks5XReq:    Socks5XReq{},
	TypeSocks5XResp:   Socks5XResp{},
}

type Login struct {
	Version string `json:"version,omitempty"`
}

type LoginResp struct {
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

type Ping struct {
}

type Pong struct {
}

type NewWorkConn struct {
}

type ReqWorkConn struct {
	Count int32 `json:"count,omitempty"`
}

type StartWorkConn struct {
}

type Socks5XReq struct {
	TargetAddr string `json:"target_addr,omitempty"`
}

type Socks5XResp struct {
	Rep byte `json:"rep,omitempty"`
}
