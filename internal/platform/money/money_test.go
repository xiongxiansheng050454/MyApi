package money

import "testing"

func TestRoundHelpers(t *testing.T) {
	if got := Round8(1.123456789); got != 1.12345679 {
		t.Fatalf("Round8 = %v", got)
	}
	if got := RoundMicro(1.5); got != 150000000 {
		t.Fatalf("RoundMicro = %d", got)
	}
	if got := AmountToMicro(2.5); got != 250000000 {
		t.Fatalf("AmountToMicro = %d", got)
	}
	if got := MicroToAmount(250000000); got != 2.5 {
		t.Fatalf("MicroToAmount = %v", got)
	}
}

func TestParseNonNegative(t *testing.T) {
	if _, ok := ParseNonNegative("-1"); ok {
		t.Fatal("negative should be rejected")
	}
	if _, ok := ParseNonNegative("abc"); ok {
		t.Fatal("non-number should be rejected")
	}
	if v, ok := ParseNonNegative("1.25"); !ok || v != 1.25 {
		t.Fatalf("parse = %v ok=%v", v, ok)
	}
}

func TestParsePositive(t *testing.T) {
	if _, err := ParsePositive("0"); err == nil {
		t.Fatal("zero should be rejected")
	}
	if v, err := ParsePositive("3"); err != nil || v != 3 {
		t.Fatalf("parse = %v err=%v", v, err)
	}
}
