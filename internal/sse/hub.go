package sse

import (
	"log"
	"sync"
)

type Subscriber chan []byte

type Hub struct {
	mu          sync.Mutex
	subscribers map[Subscriber]bool
}

var hub = &Hub{
	subscribers: make(map[Subscriber]bool),
}

// Subscribe 新增一个客户端
func Subscribe() Subscriber {
	ch := make(Subscriber, 10)
	hub.mu.Lock()
	defer hub.mu.Unlock()
	hub.subscribers[ch] = true
	return ch
}

// Unsubscribe 断开连接
func Unsubscribe(ch Subscriber) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	delete(hub.subscribers, ch)
	close(ch)
}

// Broadcast 推送消息给所有连接
func Broadcast(data []byte) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	for ch := range hub.subscribers {
		select {
		case ch <- data:
		default:
			// 队列满则剔除
			log.Println("subscriber channel full, removing")
			delete(hub.subscribers, ch)
			close(ch)
		}
	}
}
