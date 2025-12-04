package state

import "testing"

func TestIntegration(t *testing.T) {
	processor := NewMessageProcessor()

	omitChan := make(chan int64)

	processor.PutChat(1)
	processor.PutChat(2)
	processor.PutChat(3)
	processor.PutChat(1)

	go processor.Run(t.Context(), omitChan)

	if processor.GetChat() != 1 {
		t.Error("get chat return not expected result")
	}

	if processor.GetChat() != 2 {
		t.Error("get chat return not expected result")
	}

	if processor.GetChat() != 3 {
		t.Error("get chat return not expected result")
	}

	if processor.GetChat() != 0 {
		t.Error("get chat return not expected result")
	}

	omitChan <- 1

	if processor.GetChat() != 1 {
		t.Error("get chat return not expected result")
	}
}
