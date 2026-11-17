package healdata

// Минута проверена до разбора массива. Считываем только активный префикс:
// за счётчиком могут лежать ненулевые байты от предыдущей записи.
func (l arrayLayout) minute(b []byte, i int) uint16 { return word(b, l.minutes+2*i) }

func (d *Day) checkStep(l arrayLayout, i int, prev, cur uint16, watchGap bool) {
	switch {
	case cur < prev:
		d.warn(TimeReversal, l.id, l.minutes+2*i, i)
	case watchGap && cur-prev > 10:
		// Разрыв в отсчётах не определяет точного времени выключения аппарата.
		d.warn(SampleGap, l.id, l.minutes+2*i, i)
	}
}

func (d *Day) seriesCount(l arrayLayout) int {
	n := int(word(d.raw, l.count))
	switch n {
	case 0:
		d.warn(EmptySeries, l.id, l.count, -1)
	case l.capacity:
		d.warn(SeriesAtCapacity, l.id, l.count, -1)
	}
	return n
}

func (d *Day) readSeries() {
	l := pressureLayout
	d.Pressure = make([]PressurePoint, d.seriesCount(l))
	for i := range d.Pressure {
		p := PressurePoint{l.minute(d.raw, i), word(d.raw, l.values+2*i), word(d.raw, secondPressureOffset+2*i)}
		if i > 0 {
			d.checkStep(l, i, d.Pressure[i-1].Minute, p.Minute, false)
		}
		d.Pressure[i] = p
	}

	for ch, l := range channelLayouts {
		points := make([]Point, d.seriesCount(l))
		for i := range points {
			off := l.values + l.width*i
			raw := uint16(d.raw[off])
			if l.width == 2 {
				raw = word(d.raw, off)
			}
			points[i] = Point{l.minute(d.raw, i), raw}
			if i > 0 {
				d.checkStep(l, i, points[i-1].Minute, points[i].Minute, true)
			}
		}
		d.Channels[ch] = Series{ID: l.id, Points: points}
		// Сравнение с первой осью даёт диагностику, но не меняет ось канала.
		if ch > 0 {
			first := d.Channels[0].Points
			if len(first) != len(points) {
				d.warn(AxesDiffer, l.id, l.count, -1)
			} else {
				for i, p := range points {
					if p.Minute != first[i].Minute {
						d.warn(AxesDiffer, l.id, l.minutes+2*i, i)
						break
					}
				}
			}
		}
	}
}
