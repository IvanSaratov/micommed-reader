package healdata

// Отдельные счётчики в сводке позволяют проверить полноту списка событий.
var tallyFields = [...]struct {
	code   uint8
	offset int
	id     FieldID
}{
	{102, 0x0e04, "counter_102"}, {103, 0x0e06, "counter_103"},
	{101, 0x0e08, "counter_101"}, {104, 0x0e0a, "counter_104"},
}

func (d *Day) readEvents() {
	l := eventLayout
	n := int(word(d.raw, l.count))
	if n == l.capacity {
		d.warn(EventsAtCapacity, l.id, l.count, -1)
	}
	d.Events = make([]Event, n)
	var counts [256]uint16
	for i := range d.Events {
		code := d.raw[l.values+i]
		minute := l.minute(d.raw, i)
		d.Events[i] = Event{Minute: minute, Code: code}
		counts[code]++
		if i > 0 {
			d.checkStep(l, i, d.Events[i-1].Minute, minute, false)
		}
	}
	d.Counters = make(map[uint8]uint16, len(tallyFields))
	for _, c := range tallyFields {
		stored := word(d.raw, c.offset)
		d.Counters[c.code] = stored
		if stored != counts[c.code] {
			d.warn(CounterMismatch, c.id, c.offset, -1)
		}
	}
}
