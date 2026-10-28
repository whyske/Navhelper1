package timecalc

import "testing"

func press(k *Keypad, keys ...string) string {
	var last string
	for _, s := range keys {
		if r := k.Press(s); r != "" {
			last = r
		}
	}
	return last
}

func TestKeypad(t *testing.T) {
	cases := []struct {
		keys []string
		want string
	}{
		{[]string{"1", "5", "0", ":", "3", "0", "+", "2", ":", "1", "5", "="}, "152:45"},
		{[]string{"2", "2", ":", "4", "8", "+", "4", ":", "3", "8", "="}, "27:26"},
		{[]string{"5", "−", "7", ":", "3", "0", "="}, "-2:30"},
		{[]string{"1", "0", ":", "0", "0", "×", "3", "="}, "30:00"},
		{[]string{"1", "0", ":", "0", "0", "÷", "4", "="}, "2:30"},
		{[]string{"1", ":", "0", "0", "÷", "7", "="}, "0:08:34"},
		{[]string{"1", ",", "5", "+", "0", ":", "4", "5", "="}, "2:15"},
		{[]string{"1", ":", "3", "0", "+", "1", ":", "0", "0", "+", "0", ":", "3", "0", "="}, "3:00"},
	}
	for _, c := range cases {
		k := &Keypad{}
		press(k, c.keys...)
		if k.Err != "" {
			t.Errorf("%v: ошибка %s", c.keys, k.Err)
		}
		if got := k.Display(); got != c.want {
			t.Errorf("%v: получили %s, ждали %s", c.keys, got, c.want)
		}
	}
	k := &Keypad{}
	if r := press(k, "1", "5", "0", ":", "3", "0", "+", "2", ":", "1", "5", "="); r != "150:30 + 2:15 = 152:45" {
		t.Errorf("выражение: %q", r)
	}
	k = &Keypad{}
	press(k, "1", "0", "÷", "0", "=")
	if k.Err == "" {
		t.Error("деление на ноль не поймано")
	}
	k = &Keypad{}
	press(k, "1", ":", "7", "5")
	if d := k.Display(); d != "1:75" {
		t.Errorf("ввод: %s", d)
	}
	press(k, "+")
	if k.Err == "" {
		t.Error("75 минут должно быть ошибкой")
	}
}
