package state

import "sync"

type QueueMember struct {
	ChatID int64
	Next   *QueueMember
}

type QueueManager struct {
	mu            sync.RWMutex
	currentMember *QueueMember
	lastMember    *QueueMember
}

func NewQueueManager() *QueueManager {
	return &QueueManager{
		currentMember: nil,
		lastMember:    nil,
	}
}

func (q *QueueManager) Push(chatID int64) {
	q.mu.Lock()
	defer q.mu.Unlock()

	queue := &QueueMember{
		ChatID: chatID,
	}

	if q.currentMember == nil {
		q.currentMember = queue
	}

	if q.lastMember != nil {
		q.lastMember.Next = queue
	}

	q.lastMember = queue
}

func (q *QueueManager) Pop() *QueueMember {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.currentMember
}

func (q *QueueManager) Omit(chatID int64) {
	q.mu.Lock()
	defer q.mu.Unlock()

	queue := q.currentMember

	if queue == nil {
		return
	}

	if queue.ChatID == chatID {
		if queue == q.lastMember {
			q.lastMember = nil
		}

		q.currentMember = queue.Next
		return
	}

	for {
		if queue == nil {
			return
		}

		nextQueue := queue.Next

		if nextQueue == nil {
			return
		}

		if nextQueue.ChatID != chatID {
			queue = nextQueue
			continue
		}

		queue.Next = nextQueue.Next

		if q.lastMember == nextQueue {
			q.lastMember = queue.Next
		}

		if q.lastMember == nil {
			q.lastMember = queue
		}
	}
}
