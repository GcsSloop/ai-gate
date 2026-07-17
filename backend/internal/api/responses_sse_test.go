package api

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

type disconnectAfterResponsesCompletedWriter struct {
	header             http.Header
	completedLineSeen  bool
	completedEventDone bool
}

func (w *disconnectAfterResponsesCompletedWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *disconnectAfterResponsesCompletedWriter) Write(payload []byte) (int, error) {
	line := string(payload)
	if strings.Contains(line, `"type":"response.completed"`) {
		w.completedLineSeen = true
		return len(payload), nil
	}
	if w.completedLineSeen && line == "\n" {
		w.completedEventDone = true
		return len(payload), nil
	}
	if w.completedEventDone && strings.Contains(line, "data: [DONE]") {
		return 0, errors.New("client disconnected after response.completed")
	}
	return len(payload), nil
}

func (w *disconnectAfterResponsesCompletedWriter) WriteHeader(int) {}

func TestConsumeResponsesStreamErrorsWhenEOFBeforeCompleted(t *testing.T) {
	t.Parallel()

	var got strings.Builder
	err := consumeResponsesStream(strings.NewReader(
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\n",
	), func(delta string) error {
		got.WriteString(delta)
		return nil
	}, nil)

	if err == nil {
		t.Fatalf("consumeResponsesStream err = nil, want non-nil when stream ends before response.completed; got text=%q", got.String())
	}
	if !strings.Contains(err.Error(), "response.completed") {
		t.Fatalf("consumeResponsesStream err = %q, want mention response.completed", err.Error())
	}
}

func TestConsumeResponsesStreamAcceptsDataWithoutSpaceAndCRLF(t *testing.T) {
	t.Parallel()

	var got strings.Builder
	body := strings.NewReader(
		"event: response.output_text.delta\r\n" +
			"data:{\"type\":\"response.output_text.delta\",\"delta\":\"he\"}\r\n" +
			"\r\n" +
			"data:{\"type\":\"response.output_text.delta\",\"delta\":\"llo\"}\r\n" +
			"\r\n" +
			"event: response.completed\r\n" +
			"data:{\"type\":\"response.completed\",\"response\":{\"id\":\"r1\"}}\r\n" +
			"\r\n",
	)
	err := consumeResponsesStream(body, func(delta string) error {
		got.WriteString(delta)
		return nil
	}, nil)
	if err != nil {
		t.Fatalf("consumeResponsesStream returned error: %v", err)
	}
	if got.String() != "hello" {
		t.Fatalf("consumeResponsesStream got %q, want %q", got.String(), "hello")
	}
}

func TestCopyResponseStreamWithObserverIgnoresDisconnectAfterResponsesCompleted(t *testing.T) {
	t.Parallel()

	w := &disconnectAfterResponsesCompletedWriter{}
	err := copyResponseStreamWithObserver(w, strings.NewReader(
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r1\"}}\n\n"+
			"data: [DONE]\n\n",
	), nil)
	if err != nil {
		t.Fatalf("copyResponseStreamWithObserver returned error: %v, want success after terminal response", err)
	}
}
