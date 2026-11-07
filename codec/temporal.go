package codec

import (
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"slices"
	"strconv"
)

// convertTemporal returns the value of a's mirror, a date, time or duration
// datatype struct, that form denotes, or the error for a form that is not of
// a's lexical grammar (Datatypes §3.3.6.2, §3.3.7.2–§3.3.14.2, §3.4.26.2–
// §3.4.28.2, the fragments of §D.2.2) or that denotes no value of a's value
// space: a property outside its range (§D.2.1: month 1 to 12, day 1 to
// ·daysInMonth·, hour 0 to 23, minute 0 to 59, second below 60, timezoneOffset
// −840 to 840), so an hour of 24, which the lexical mapping normalizes away
// (§E.3.1), included, dateTimeStamp's absent timezoneOffset (§3.4.28), and a
// yearMonthDuration's seconds or a dayTimeDuration's months other than 0
// (§3.4.26, §3.4.27). Each property is the local value the form shows, never
// converted to UTC (§D.2.1), and a duration's Months and Seconds both take the
// form's one sign (§3.3.6.1). Any other a panics, since convert delegates to
// it every atomic convert does not name.
func convertTemporal(a atomic, form string) (reflect.Value, error) {
	c := &scanner{s: form}
	var v any
	var err error
	switch a {
	case atomicDateTime, atomicDateTimeStamp:
		v, err = c.dateTime(a == atomicDateTimeStamp)
	case atomicTime:
		v, err = c.time()
	case atomicDate:
		v, err = c.date()
	case atomicGYearMonth:
		v, err = c.gYearMonth()
	case atomicGYear:
		v, err = c.gYear()
	case atomicGMonthDay:
		v, err = c.gMonthDay()
	case atomicGDay:
		v, err = c.gDay()
	case atomicGMonth:
		v, err = c.gMonth()
	case atomicDuration, atomicDayTimeDuration, atomicYearMonthDuration:
		v, err = c.duration(a)
	default:
		// convert delegates every atomic it does not convert itself; of those,
		// atomicString's value is the normalized lexical itself, and
		// atomicQName's expandedName's.
		panic(fmt.Sprintf("codec: atomic %s has no conversion", a.bare()))
	}
	if err != nil {
		return reflect.Value{}, err
	}
	if !c.done() {
		return reflect.Value{}, fmt.Errorf("%q follows the value at byte %d", c.s[c.i:], c.i)
	}
	return reflect.ValueOf(v), nil
}

// scanner reads a date, time or duration form s from byte offset i.
type scanner struct {
	s string
	i int
}

// done reports whether c has read the whole form.
func (c *scanner) done() bool { return c.i == len(c.s) }

// lit consumes b where it is the next byte, and reports whether it was.
func (c *scanner) lit(b byte) bool {
	if c.i < len(c.s) && c.s[c.i] == b {
		c.i++
		return true
	}
	return false
}

// expect consumes b, or returns the error naming what is there instead.
func (c *scanner) expect(b byte) error {
	if c.lit(b) {
		return nil
	}
	if c.done() {
		return fmt.Errorf("the form ends where %q is expected", b)
	}
	return fmt.Errorf("%q at byte %d where %q is expected", c.s[c.i], c.i, b)
}

// digits consumes the longest run of ASCII digits and returns it.
func (c *scanner) digits() string {
	start := c.i
	for c.i < len(c.s) && c.s[c.i] >= '0' && c.s[c.i] <= '9' {
		c.i++
	}
	return c.s[start:c.i]
}

// two consumes exactly two digits and returns their value as the property
// name, read by strconv at uint8's width, an error unless it lies between lo
// and hi inclusive.
func (c *scanner) two(name string, lo, hi uint64) (uint8, error) {
	start := c.i
	d := c.digits()
	if len(d) != 2 {
		return 0, fmt.Errorf("the %s at byte %d is %q, not two digits", name, start, d)
	}
	n, err := strconv.ParseUint(d, 10, 8)
	if err != nil {
		return 0, err
	}
	if n < lo || n > hi {
		return 0, fmt.Errorf("the %s %d is outside %d to %d (§D.2.1)", name, n, lo, hi)
	}
	return uint8(n), nil
}

// year consumes a yearFrag (§D.2.2): an optional '-' and four or more digits,
// a leading zero only in exactly four.
func (c *scanner) year() (big.Int, error) {
	start := c.i
	c.lit('-')
	d := c.digits()
	if len(d) < 4 || (len(d) > 4 && d[0] == '0') {
		return big.Int{}, fmt.Errorf("the year at byte %d is %q, no yearFrag", start, c.s[start:c.i])
	}
	var y big.Int
	if _, ok := y.SetString(c.s[start:c.i], 10); !ok {
		return big.Int{}, fmt.Errorf("big.Int does not read the year %q", c.s[start:c.i])
	}
	return y, nil
}

// second consumes a secondFrag (§D.2.2): two digits and an optional '.' with
// one or more digits, a value below 60.
func (c *scanner) second() (big.Rat, error) {
	start := c.i
	if len(c.digits()) != 2 {
		return big.Rat{}, fmt.Errorf("the second at byte %d is not two digits", start)
	}
	if c.lit('.') && c.digits() == "" {
		return big.Rat{}, fmt.Errorf("the second at byte %d has no digit after its '.'", start)
	}
	var s big.Rat
	if _, ok := s.SetString(c.s[start:c.i]); !ok {
		return big.Rat{}, fmt.Errorf("big.Rat does not read the second %q", c.s[start:c.i])
	}
	if s.Cmp(big.NewRat(60, 1)) >= 0 {
		return big.Rat{}, fmt.Errorf("the second %s is not below 60 (§D.2.1)", c.s[start:c.i])
	}
	return s, nil
}

// timezone consumes an optional timezoneFrag (§D.2.2) ending the form and
// returns its timezoneOffset in minutes, nil where the form ends without one:
// 'Z' is 0, and a sign, two-digit hours and two-digit minutes are at most
// 14:00 either way.
func (c *scanner) timezone() (*int16, error) {
	if c.done() {
		return nil, nil
	}
	if c.lit('Z') {
		return new(int16), nil
	}
	sign := int16(1)
	switch {
	case c.lit('-'):
		sign = -1
	case c.lit('+'):
	default:
		return nil, fmt.Errorf("%q at byte %d is no timezoneFrag", c.s[c.i], c.i)
	}
	h, err := c.two("timezone hour", 0, 14)
	if err != nil {
		return nil, err
	}
	if err := c.expect(':'); err != nil {
		return nil, err
	}
	m, err := c.two("timezone minute", 0, 59)
	if err != nil {
		return nil, err
	}
	if h == 14 && m != 0 {
		return nil, errors.New("the timezoneOffset is beyond 840 minutes (§D.2.1)")
	}
	off := sign * (int16(h)*60 + int16(m))
	return &off, nil
}

// yearMonthDay consumes year '-' month '-' day.
func (c *scanner) yearMonthDay() (big.Int, uint8, uint8, error) {
	y, err := c.year()
	if err != nil {
		return big.Int{}, 0, 0, err
	}
	m, d, err := c.monthDay(&y)
	return y, m, d, err
}

// monthDay consumes '-' month '-' day and returns the month and the day, the
// day at most ·daysInMonth· of year, nil for an absent one.
func (c *scanner) monthDay(year *big.Int) (uint8, uint8, error) {
	if err := c.expect('-'); err != nil {
		return 0, 0, err
	}
	m, err := c.two("month", 1, 12)
	if err != nil {
		return 0, 0, err
	}
	if err := c.expect('-'); err != nil {
		return 0, 0, err
	}
	d, err := c.two("day", 1, uint64(daysInMonth(year, m)))
	return m, d, err
}

// clock consumes hour ':' minute ':' second, the hour at most 23.
func (c *scanner) clock() (uint8, uint8, big.Rat, error) {
	h, err := c.two("hour", 0, 23)
	if err != nil {
		return 0, 0, big.Rat{}, err
	}
	if err := c.expect(':'); err != nil {
		return 0, 0, big.Rat{}, err
	}
	m, err := c.two("minute", 0, 59)
	if err != nil {
		return 0, 0, big.Rat{}, err
	}
	if err := c.expect(':'); err != nil {
		return 0, 0, big.Rat{}, err
	}
	s, err := c.second()
	return h, m, s, err
}

// dateTime consumes a dateTime form (§3.3.7.2) and returns its mirror, or
// where stamp is true a dateTimeStamp's, whose timezoneOffset is required
// (§3.4.28).
func (c *scanner) dateTime(stamp bool) (any, error) {
	y, mo, d, err := c.yearMonthDay()
	if err != nil {
		return nil, err
	}
	if err := c.expect('T'); err != nil {
		return nil, err
	}
	h, mi, s, err := c.clock()
	if err != nil {
		return nil, err
	}
	tz, err := c.timezone()
	if err != nil {
		return nil, err
	}
	if !stamp {
		return dateTimeMirror{Year: y, Month: mo, Day: d, Hour: h, Minute: mi, Second: s, TimezoneOffset: tz}, nil
	}
	if tz == nil {
		return nil, errors.New("a dateTimeStamp's timezoneOffset is absent (§3.4.28)")
	}
	return dateTimeStampMirror{Year: y, Month: mo, Day: d, Hour: h, Minute: mi, Second: s, TimezoneOffset: *tz}, nil
}

// time consumes a time form (§3.3.8.2) and returns its mirror.
func (c *scanner) time() (any, error) {
	h, mi, s, err := c.clock()
	if err != nil {
		return nil, err
	}
	tz, err := c.timezone()
	return timeMirror{Hour: h, Minute: mi, Second: s, TimezoneOffset: tz}, err
}

// date consumes a date form (§3.3.9.2) and returns its mirror.
func (c *scanner) date() (any, error) {
	y, mo, d, err := c.yearMonthDay()
	if err != nil {
		return nil, err
	}
	tz, err := c.timezone()
	return dateMirror{Year: y, Month: mo, Day: d, TimezoneOffset: tz}, err
}

// gYearMonth consumes a gYearMonth form (§3.3.10.2) and returns its mirror.
func (c *scanner) gYearMonth() (any, error) {
	y, err := c.year()
	if err != nil {
		return nil, err
	}
	if err := c.expect('-'); err != nil {
		return nil, err
	}
	mo, err := c.two("month", 1, 12)
	if err != nil {
		return nil, err
	}
	tz, err := c.timezone()
	return gYearMonthMirror{Year: y, Month: mo, TimezoneOffset: tz}, err
}

// gYear consumes a gYear form (§3.3.11.2) and returns its mirror.
func (c *scanner) gYear() (any, error) {
	y, err := c.year()
	if err != nil {
		return nil, err
	}
	tz, err := c.timezone()
	return gYearMirror{Year: y, TimezoneOffset: tz}, err
}

// gMonthDay consumes a gMonthDay form (§3.3.12.2), whose day is at most
// ·daysInMonth· of an absent year, and returns its mirror.
func (c *scanner) gMonthDay() (any, error) {
	if err := c.expect('-'); err != nil {
		return nil, err
	}
	mo, d, err := c.monthDay(nil)
	if err != nil {
		return nil, err
	}
	tz, err := c.timezone()
	return gMonthDayMirror{Month: mo, Day: d, TimezoneOffset: tz}, err
}

// gDay consumes a gDay form (§3.3.13.2) and returns its mirror.
func (c *scanner) gDay() (any, error) {
	for range 3 {
		if err := c.expect('-'); err != nil {
			return nil, err
		}
	}
	d, err := c.two("day", 1, 31)
	if err != nil {
		return nil, err
	}
	tz, err := c.timezone()
	return gDayMirror{Day: d, TimezoneOffset: tz}, err
}

// gMonth consumes a gMonth form (§3.3.14.2) and returns its mirror.
func (c *scanner) gMonth() (any, error) {
	for range 2 {
		if err := c.expect('-'); err != nil {
			return nil, err
		}
	}
	mo, err := c.two("month", 1, 12)
	if err != nil {
		return nil, err
	}
	tz, err := c.timezone()
	return gMonthMirror{Month: mo, TimezoneOffset: tz}, err
}

// durationPart is one designator of a durationLexicalRep (§3.3.6.2) and what
// its unit weighs in ·months· and in ·seconds· (§E.2 ·durationMap·).
type durationPart struct {
	designator byte
	months     int64
	seconds    int64
}

// The designators of a durationLexicalRep in order, before its 'T' and after
// it.
var (
	dateParts = []durationPart{{'Y', 12, 0}, {'M', 1, 0}, {'D', 0, 86400}}
	timeParts = []durationPart{{'H', 0, 3600}, {'M', 0, 60}, {'S', 0, 1}}
)

// duration consumes a durationLexicalRep (§3.3.6.2) and returns the mirror of
// a, duration, dayTimeDuration or yearMonthDuration: ·months· and ·seconds·
// as ·durationMap· sums them (§E.2), both negated by a leading '-'.
func (c *scanner) duration(a atomic) (any, error) {
	negative := c.lit('-')
	if err := c.expect('P'); err != nil {
		return nil, err
	}
	var months big.Int
	var seconds big.Rat
	n, err := c.durationHalf(dateParts, &months, &seconds)
	if err != nil {
		return nil, err
	}
	if c.lit('T') {
		t, err := c.durationHalf(timeParts, &months, &seconds)
		if err != nil {
			return nil, err
		}
		if t == 0 {
			return nil, errors.New("a duration's 'T' is followed by no hour, minute or second")
		}
		n += t
	}
	if n == 0 {
		return nil, errors.New("a duration has no year, month, day, hour, minute or second")
	}
	if negative {
		months.Neg(&months)
		seconds.Neg(&seconds)
	}
	switch a {
	case atomicDayTimeDuration:
		if months.Sign() != 0 {
			return nil, errors.New("a dayTimeDuration's months are not 0 (§3.4.27)")
		}
		return dayTimeDurationMirror{Seconds: seconds}, nil
	case atomicYearMonthDuration:
		if seconds.Sign() != 0 {
			return nil, errors.New("a yearMonthDuration's seconds are not 0 (§3.4.26)")
		}
		return yearMonthDurationMirror{Months: months}, nil
	default:
		return durationMirror{Months: months, Seconds: seconds}, nil
	}
}

// durationHalf consumes the components of one half of a durationLexicalRep up
// to its 'T' or its end, each a numeral and one of parts' designators in
// parts' order, adds each one's weight to months and seconds, and returns how
// many it read. Only a second carries a fractional part (duSecondFrag).
func (c *scanner) durationHalf(parts []durationPart, months *big.Int, seconds *big.Rat) (int, error) {
	n := 0
	for !c.done() && c.s[c.i] != 'T' {
		start := c.i
		whole := c.digits()
		point := c.lit('.')
		frac := ""
		if point {
			frac = c.digits()
		}
		if whole == "" && frac == "" {
			return 0, fmt.Errorf("%q at byte %d is no duration component", c.s[start:], start)
		}
		if c.done() {
			return 0, fmt.Errorf("the duration component %q has no designator", c.s[start:])
		}
		designator := c.s[c.i]
		c.i++
		k := slices.IndexFunc(parts, func(p durationPart) bool { return p.designator == designator })
		if k < 0 {
			return 0, fmt.Errorf("the designator %q at byte %d is out of order or of the other half", designator, c.i-1)
		}
		part := parts[k]
		parts = parts[k+1:]
		numeral := c.s[start : c.i-1]
		if point && designator != 'S' {
			return 0, fmt.Errorf("the duration component %q carries a fraction", c.s[start:c.i])
		}
		var r big.Rat
		if _, ok := r.SetString(numeral); !ok {
			return 0, fmt.Errorf("big.Rat does not read %q", numeral)
		}
		if part.months != 0 {
			months.Add(months, new(big.Int).Mul(r.Num(), big.NewInt(part.months)))
		}
		seconds.Add(seconds, r.Mul(&r, new(big.Rat).SetInt64(part.seconds)))
		n++
	}
	return n, nil
}

// daysInMonth is ·daysInMonth·(y, m) (Datatypes §E.3.2 f-daysInMonth): 28 for
// February of a year not divisible by 4, or divisible by 100 and not by 400,
// 29 for February of any other year or of an absent one, nil, 30 for April,
// June, September and November, and 31 otherwise.
func daysInMonth(year *big.Int, m uint8) uint8 {
	switch m {
	case 2:
		if year == nil || divisible(year, 4) && (!divisible(year, 100) || divisible(year, 400)) {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	}
	return 31
}

// divisible reports whether n is divisible by d.
func divisible(n *big.Int, d int64) bool {
	return new(big.Int).Rem(n, big.NewInt(d)).Sign() == 0
}
