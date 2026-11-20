package quantity

import (
	"encoding/json"
	"testing"
)

func TestQuantity(t *testing.T) {
	q := MustParse("12.5")
	if q.Thousandths() != 12500 || q.String() != "12.5" || q.Format() != "12,5" {
		t.Errorf("12.5: %d %q %q", q.Thousandths(), q.String(), q.Format())
	}
	if FromInt(1234).Format() != "1.234" {
		t.Errorf("Format = %q", FromInt(1234).Format())
	}
	if !FromInt(3).IsWhole() || MustParse("2.5").IsWhole() {
		t.Error("IsWhole")
	}
	// 2,5 caixas × 18,2 kg = 45,5 kg
	if kg, _ := MustParse("2.5").Mul(MustParse("18.2")); kg != MustParse("45.5") {
		t.Errorf("Mul = %s", kg)
	}
	// Desmembrar: 30 ovos - 20 = 10
	if FromInt(30).Sub(FromInt(20)) != FromInt(10) {
		t.Error("Sub")
	}
	if _, err := Parse("1.2345"); err == nil {
		t.Error("4 decimals should fail")
	}
}

func TestQuantityJSON(t *testing.T) {
	var v struct {
		Q  Quantity  `json:"q"`
		QP *Quantity `json:"qp"`
	}
	if err := json.Unmarshal([]byte(`{"q":"0.125","qp":null}`), &v); err != nil || v.Q != 125 || v.QP != nil {
		t.Fatalf("unmarshal: %v %+v", err, v)
	}
	b, _ := json.Marshal(v)
	if string(b) != `{"q":"0.125","qp":null}` {
		t.Errorf("marshal = %s", b)
	}
	if err := json.Unmarshal([]byte(`{"q":0.125}`), &v); err == nil {
		t.Error("JSON number should be rejected (must be a string)")
	}
}
