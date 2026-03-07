package api

import (
	"strings"
	"testing"
)

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

