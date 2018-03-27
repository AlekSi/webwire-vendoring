package main

import (
	"github.com/gorilla/websocket"
	"github.com/qbeon/webwire-go"
)

func main() {
	conn, _, _ := websocket.NewClient(nil, nil, nil, 0, 0)
	_ = webwire.NewClientAgent(conn, "", nil)
}
