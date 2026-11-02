package auth

import "testing"

func TestPasswordRoundTrip(t *testing.T) {
	h, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := CheckPassword(h, "correct horse"); err != nil || !ok {
		t.Fatalf("matching password rejected: ok=%v err=%v", ok, err)
	}
	if ok, _ := CheckPassword(h, "wrong"); ok {
		t.Fatal("wrong password accepted")
	}
	h2, _ := HashPassword("correct horse")
	if h == h2 {
		t.Fatal("hashes should be salted")
	}
	if _, err := CheckPassword("$bcrypt$nope", "x"); err == nil {
		t.Fatal("malformed hash should error")
	}
}
