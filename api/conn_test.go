package api_test

import (
	"net"
	"testing"

	"github.com/frostyard/frostnotes/api"
)

// The client's own errors name notesd.
func TestClientErrorsNameNotesd(t *testing.T) {
	server, client := net.Pipe()
	c := api.NewClient(client)
	_ = server.Close()
	<-c.Done()
	if err := c.Err(); err == nil || err.Error() != "notesd: connection closed" {
		t.Fatalf("Err after the server hung up = %v, want notesd: connection closed", err)
	}
}
