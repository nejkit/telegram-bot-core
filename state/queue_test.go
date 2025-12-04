package state

import "testing"

func TestAddQueue(t *testing.T) {
	manager := NewQueueManager()

	manager.Push(1)
	manager.Push(2)

	first := manager.Pop()
	second := first.Next

	if first.ChatID != 1 {
		t.Error("first.ChatID != 1")
	}

	if second.ChatID != 2 {
		t.Error("second.ChatID != 2")
	}

	if manager.lastMember != second {
		t.Error("manager.lastMember != second")
	}

	if manager.currentMember != first {
		t.Error("manager.currentMember != first")
	}

	if second.Next != nil {
		t.Error("second.Next != nil")
	}
}

func TestDeleteFirstElement(t *testing.T) {
	manager := NewQueueManager()

	manager.Push(1)
	manager.Push(2)

	manager.Omit(1)

	second := manager.Pop()

	if second.ChatID != 2 {
		t.Error("second.ChatID != 2")
	}

	if manager.currentMember != second {
		t.Error("manager.currentMember != second")
	}

	if second.Next != nil {
		t.Error("second.Next != nil")
	}

	if manager.lastMember != second {
		t.Error("manager.lastMember != second")
	}
}

func TestDeleteLastElement(t *testing.T) {
	manager := NewQueueManager()

	manager.Push(1)
	manager.Push(2)

	manager.Omit(2)

	first := manager.Pop()

	if first.ChatID != 1 {
		t.Error("first.ChatID != 1")
	}

	if manager.currentMember != first {
		t.Error("manager.currentMember != first")
	}

	if manager.lastMember != first {
		t.Error("manager.lastMember != first")
	}

	if first.Next != nil {
		t.Error("first.Next != nil")
	}
}

func TestDeleteMiddleElement(t *testing.T) {
	manager := NewQueueManager()

	manager.Push(1)
	manager.Push(2)
	manager.Push(3)

	manager.Omit(2)

	first := manager.Pop()
	third := first.Next

	if third.ChatID != 3 {
		t.Error("third.ChatID != 3")
	}

	if first.ChatID != 1 {
		t.Error("first.ChatID != 1")
	}

	if manager.currentMember != first {
		t.Error("manager.currentMember != first")
	}

	if manager.lastMember != third {
		t.Error("manager.lastMember != third")
	}

	if first.Next != third {
		t.Error("first.Next != third")
	}

	if third.Next != nil {
		t.Error("third.Next != nil")
	}
}
