package tailnet

import "testing"

const sample = `{
  "Self": {"ID":"n1","HostName":"mac","DNSName":"mac.tail1.ts.net.","OS":"macOS","Online":true,"UserID":42},
  "Peer": {"k":{"ID":"n2","HostName":"box","DNSName":"box.tail1.ts.net.","OS":"linux","Online":true,"UserID":42}},
  "User": {"42":{"LoginName":"me@example.com"}}
}`

func TestOwnerLoginAndHostName(t *testing.T) {
	s, err := ParseStatus([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	login, err := s.OwnerLogin()
	if err != nil || login != "me@example.com" {
		t.Fatalf("OwnerLogin = %q, %v", login, err)
	}
	if got := s.HostName(); got != "mac.tail1.ts.net" {
		t.Fatalf("HostName = %q", got)
	}
}

func TestOwnerLoginMissingUser(t *testing.T) {
	s, _ := ParseStatus([]byte(`{"Self":{"UserID":7},"User":{}}`))
	if _, err := s.OwnerLogin(); err == nil {
		t.Fatal("want error for unknown user")
	}
}
