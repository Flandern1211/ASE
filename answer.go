package Agent

import "sync"

// 用户session
type UserSession struct {
	sessionId int
	username  string
}
type AgentState struct {
	session map[string][]*UserSession //用户会话
	mu      sync.RWMutex
}
