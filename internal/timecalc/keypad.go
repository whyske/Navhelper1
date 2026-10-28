package timecalc

import (
	"fmt"
	"strconv"
	"strings"
)

// Keypad — движок кнопочного калькулятора времени (как у обычного
// калькулятора: ввод операнда, операция, «=»). Вычисления идут слева
// направо, без ограничения 24 часами.
//
// Ввод операнда для «+» и «−»: «150:30» = 150 ч 30 мин, «1:05:20» = 1 ч 05 мин
// 20 с, число без двоеточия — часы («5» = 5 ч). Для «×» и «÷» операнд —
// обычное число (можно дробное: «1,5»).
type Keypad struct {
	acc    Duration // накопленный результат
	hasAcc bool
	op     Op   // ожидающая операция
	hasOp  bool
	cur    string // набираемый операнд
	expr   string // выражение для верхней строки дисплея
	done   bool   // только что нажали «=»
	Err    string
}

const (
	OpMul Op = 100 + iota
	OpDiv
)

func opSym(o Op) string {
	switch o {
	case OpAdd:
		return "+"
	case OpSub:
		return "−"
	case OpMul:
		return "×"
	case OpDiv:
		return "÷"
	}
	return "?"
}

// numericOperand — следующий операнд — число, а не время.
func (k *Keypad) numericOperand() bool { return k.hasOp && (k.op == OpMul || k.op == OpDiv) }

// Press обрабатывает нажатие: "0".."9", ":", ",", "⌫", "C", "±", "+", "−", "×", "÷", "=".
// Возвращает выражение «… = результат», если было вычисление по «=».
func (k *Keypad) Press(key string) string {
	k.Err = ""
	switch key {
	case "C":
		*k = Keypad{}
		return ""
	case "⌫":
		if k.cur != "" {
			_, size := lastRune(k.cur)
			k.cur = k.cur[:len(k.cur)-size]
		}
		return ""
	case "±":
		if strings.HasPrefix(k.cur, "-") {
			k.cur = k.cur[1:]
		} else if k.cur != "" {
			k.cur = "-" + k.cur
		} else if k.hasAcc && !k.hasOp {
			k.acc = -k.acc
		}
		return ""
	case ":":
		if k.numericOperand() || strings.Count(k.cur, ":") >= 2 || strings.Contains(k.cur, ",") {
			return ""
		}
		k.startTyping()
		if k.cur == "" || k.cur == "-" {
			k.cur += "0"
		}
		k.cur += ":"
		return ""
	case ",":
		if strings.Contains(k.cur, ",") || strings.Contains(k.cur, ":") {
			return ""
		}
		k.startTyping()
		if k.cur == "" || k.cur == "-" {
			k.cur += "0"
		}
		k.cur += ","
		return ""
	case "+", "−", "×", "÷":
		var op Op
		switch key {
		case "+":
			op = OpAdd
		case "−":
			op = OpSub
		case "×":
			op = OpMul
		case "÷":
			op = OpDiv
		}
		if k.cur != "" {
			if !k.commit() {
				return ""
			}
		} else if !k.hasAcc {
			k.hasAcc, k.acc = true, 0
			k.expr = Duration(0).Short()
		}
		k.op, k.hasOp, k.done = op, true, false
		return ""
	case "=":
		if !k.hasOp || k.cur == "" {
			return ""
		}
		full := k.expr + " " + opSym(k.op) + " " + k.curText()
		if !k.commit() {
			return ""
		}
		k.hasOp, k.done = false, true
		res := full + " = " + k.acc.Short()
		k.expr = k.acc.Short()
		return res
	}
	if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
		k.startTyping()
		if len(strings.TrimLeft(k.cur, "-")) >= 12 {
			return ""
		}
		// После двоеточия — не более двух цифр в поле минут/секунд.
		if i := strings.LastIndex(k.cur, ":"); i >= 0 && len(k.cur)-i-1 >= 2 {
			return ""
		}
		k.cur += key
	}
	return ""
}

// startTyping — новый ввод после «=» начинает вычисление заново.
func (k *Keypad) startTyping() {
	if k.done {
		*k = Keypad{}
	}
}

// commit применяет набранный операнд к аккумулятору.
func (k *Keypad) commit() bool {
	text := k.curText()
	if !k.hasOp {
		d, err := ParseOperand(k.cur)
		if err != nil {
			k.Err = err.Error()
			return false
		}
		k.acc, k.hasAcc, k.cur, k.expr = d, true, "", text
		return true
	}
	switch k.op {
	case OpMul, OpDiv:
		n, err := strconv.ParseFloat(strings.ReplaceAll(k.cur, ",", "."), 64)
		if err != nil {
			k.Err = "введите число"
			return false
		}
		if k.op == OpDiv {
			if n == 0 {
				k.Err = "деление на ноль"
				return false
			}
			k.acc = roundDur(float64(k.acc) / n)
		} else {
			k.acc = roundDur(float64(k.acc) * n)
		}
	default:
		d, err := ParseOperand(k.cur)
		if err != nil {
			k.Err = err.Error()
			return false
		}
		if k.op == OpAdd {
			k.acc += d
		} else {
			k.acc -= d
		}
	}
	k.expr = k.expr + " " + opSym(k.op) + " " + text
	k.cur = ""
	return true
}

func roundDur(v float64) Duration {
	if v < 0 {
		return Duration(v - 0.5)
	}
	return Duration(v + 0.5)
}

// curText — набираемый операнд в читаемом виде («150:30» → «150 ч 30 мин»).
func (k *Keypad) curText() string {
	if k.numericOperand() {
		return k.cur
	}
	if d, err := ParseOperand(k.cur); err == nil {
		return d.Short()
	}
	return k.cur
}

// ParseOperand — операнд-время: «Ч», «Ч:ММ», «Ч:ММ:СС», «-Ч:ММ», «1,5» (часы).
func ParseOperand(s string) (Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return 0, fmt.Errorf("пустое значение")
	}
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var d Duration
	if strings.Contains(s, ":") {
		p := strings.Split(s, ":")
		mult := []Duration{Hour, Minute, Second}
		for i, x := range p {
			if x == "" {
				continue
			}
			v, err := strconv.ParseInt(x, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("неверное число %q", x)
			}
			if i > 0 && v > 59 {
				return 0, fmt.Errorf("%s не может быть больше 59", []string{"", "минуты", "секунды"}[i])
			}
			d += Duration(v) * mult[i]
		}
	} else {
		v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
		if err != nil {
			return 0, fmt.Errorf("неверное число %q", s)
		}
		d = roundDur(v * float64(Hour))
	}
	if neg {
		d = -d
	}
	return d, nil
}

// Display — главная строка дисплея (то, что сейчас набирается, или результат).
func (k *Keypad) Display() string {
	if k.cur != "" {
		return k.cur
	}
	if k.hasAcc {
		return k.acc.Short()
	}
	return "0:00"
}

// Hint — расшифровка текущего ввода/результата («152 ч 45 мин · 6 сут 8 ч 45 мин»).
func (k *Keypad) Hint() string {
	if k.cur != "" {
		if k.numericOperand() {
			return "множитель/делитель — число"
		}
		if d, err := ParseOperand(k.cur); err == nil {
			return d.String()
		}
		return ""
	}
	if k.hasAcc {
		s := k.acc.String()
		if k.acc.Abs() >= Day {
			s += " · " + k.acc.DaysString()
		}
		return s
	}
	return "Ч:ММ — часы и минуты, без «:» — часы"
}

// Expr — верхняя строка дисплея (история текущего вычисления).
func (k *Keypad) Expr() string {
	if k.hasOp {
		return k.expr + " " + opSym(k.op)
	}
	if k.done {
		return k.expr
	}
	return k.expr
}

// Value — текущий результат.
func (k *Keypad) Value() Duration { return k.acc }

// SetValue — продолжить вычисления с заданного значения (из истории).
func (k *Keypad) SetValue(d Duration) {
	*k = Keypad{acc: d, hasAcc: true, expr: d.Short(), done: true}
}

// Short — компактная запись «150:30» / «-5:15» / «1:05:20».
func (d Duration) Short() string {
	neg, h, m, s := d.Split()
	sign := ""
	if neg {
		sign = "-"
	}
	if s != 0 {
		return fmt.Sprintf("%s%d:%02d:%02d", sign, h, m, s)
	}
	return fmt.Sprintf("%s%d:%02d", sign, h, m)
}

func lastRune(s string) (rune, int) {
	r := []rune(s)
	if len(r) == 0 {
		return 0, 0
	}
	return r[len(r)-1], len(string(r[len(r)-1]))
}
