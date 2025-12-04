package state

import (
	"context"
	"sync"
)

type MessageProcessor struct {
	queueManager *QueueManager
	processChats map[int64]struct{}
	mu           sync.Mutex
}

func NewMessageProcessor() *MessageProcessor {
	return &MessageProcessor{
		queueManager: NewQueueManager(),
		processChats: make(map[int64]struct{}),
		mu:           sync.Mutex{},
	}
}

func (m *MessageProcessor) PutChat(chatID int64) {
	m.queueManager.Push(chatID)
}

func (m *MessageProcessor) GetChat() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	queue := m.queueManager.Pop()

	if queue == nil {
		return 0
	}

	for _, ok := m.processChats[queue.ChatID]; ok; _, ok = m.processChats[queue.ChatID] {
		queue = queue.Next

		if queue == nil {
			return 0
		}
	}

	m.processChats[queue.ChatID] = struct{}{}

	return queue.ChatID
}

func (m *MessageProcessor) Run(ctx context.Context, omitChats <-chan int64) {
	for {
		select {
		case <-ctx.Done():
			return

		case chatID, ok := <-omitChats:
			if !ok {
				return
			}

			m.mu.Lock()
			delete(m.processChats, chatID)
			m.mu.Unlock()
		}
	}
}
