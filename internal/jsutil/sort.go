package jsutil

// Sort sorts a in place with the algorithm of Array.prototype.sort in V8
// (TimSort). The minifier sorts with comparators that are not always
// consistent (they never report equality), so the order of the result depends
// on the exact sequence of comparisons, which this port reproduces. cmp
// returns a negative number when x sorts before y.
func Sort[T any](a []T, cmp func(x, y T) float64) {
	if len(a) < 2 {
		return
	}
	s := &timSort[T]{a: a, cmp: cmp, minGallop: 7}
	s.run()
}

const minGallopWins = 7

type sortRun struct {
	base, length int
}

type timSort[T any] struct {
	a         []T
	cmp       func(x, y T) float64
	minGallop int
	runs      []sortRun
}

func (s *timSort[T]) less(x, y T) bool {
	return s.cmp(x, y) < 0
}

func (s *timSort[T]) run() {
	remaining := len(s.a)
	low := 0
	minRun := minRunLength(remaining)
	for remaining != 0 {
		current := s.countAndMakeRun(low, low+remaining)
		if current < minRun {
			forced := minRun
			if remaining < forced {
				forced = remaining
			}
			s.binaryInsertionSort(low, low+current, low+forced)
			current = forced
		}
		s.runs = append(s.runs, sortRun{low, current})
		s.mergeCollapse()
		low += current
		remaining -= current
	}
	s.mergeForceCollapse()
}

func minRunLength(n int) int {
	r := 0
	for n >= 64 {
		r |= n & 1
		n >>= 1
	}
	return n + r
}

func (s *timSort[T]) countAndMakeRun(lowArg, high int) int {
	low := lowArg + 1
	if low == high {
		return 1
	}
	runLength := 2
	elementLow := s.a[low]
	descending := s.less(elementLow, s.a[low-1])
	previous := elementLow
	for idx := low + 1; idx < high; idx++ {
		current := s.a[idx]
		lessThanPrevious := s.less(current, previous)
		if descending {
			if !lessThanPrevious {
				break
			}
		} else if lessThanPrevious {
			break
		}
		previous = current
		runLength++
	}
	if descending {
		for i, j := lowArg, lowArg+runLength-1; i < j; i, j = i+1, j-1 {
			s.a[i], s.a[j] = s.a[j], s.a[i]
		}
	}
	return runLength
}

func (s *timSort[T]) binaryInsertionSort(low, startArg, high int) {
	start := startArg
	if low == startArg {
		start++
	}
	for ; start < high; start++ {
		left, right := low, start
		pivot := s.a[start]
		for left < right {
			mid := left + ((right - left) >> 1)
			if s.less(pivot, s.a[mid]) {
				right = mid
			} else {
				left = mid + 1
			}
		}
		copy(s.a[left+1:start+1], s.a[left:start])
		s.a[left] = pivot
	}
}

func (s *timSort[T]) runInvariantEstablished(n int) bool {
	if n < 2 {
		return true
	}
	return s.runs[n-2].length > s.runs[n-1].length+s.runs[n].length
}

func (s *timSort[T]) mergeCollapse() {
	for len(s.runs) > 1 {
		n := len(s.runs) - 2
		if !s.runInvariantEstablished(n+1) || !s.runInvariantEstablished(n) {
			if s.runs[n-1].length < s.runs[n+1].length {
				n--
			}
			s.mergeAt(n)
		} else if s.runs[n].length <= s.runs[n+1].length {
			s.mergeAt(n)
		} else {
			break
		}
	}
}

func (s *timSort[T]) mergeForceCollapse() {
	for len(s.runs) > 1 {
		n := len(s.runs) - 2
		if n > 0 && s.runs[n-1].length < s.runs[n+1].length {
			n--
		}
		s.mergeAt(n)
	}
}

func (s *timSort[T]) mergeAt(i int) {
	baseA, lengthA := s.runs[i].base, s.runs[i].length
	baseB, lengthB := s.runs[i+1].base, s.runs[i+1].length
	s.runs[i].length = lengthA + lengthB
	if i == len(s.runs)-3 {
		s.runs[i+1] = s.runs[i+2]
	}
	s.runs = s.runs[:len(s.runs)-1]

	k := gallopRight(s, s.a, s.a[baseB], baseA, lengthA, 0)
	baseA += k
	lengthA -= k
	if lengthA == 0 {
		return
	}
	lengthB = gallopLeft(s, s.a, s.a[baseA+lengthA-1], baseB, lengthB, lengthB-1)
	if lengthB == 0 {
		return
	}
	if lengthA <= lengthB {
		s.mergeLow(baseA, lengthA, baseB, lengthB)
	} else {
		s.mergeHigh(baseA, lengthA, baseB, lengthB)
	}
}

// gallopLeft locates the position at which key belongs in arr[base:base+length]:
// the leftmost position, so equal elements stay to the right of key.
func gallopLeft[T any](s *timSort[T], arr []T, key T, base, length, hint int) int {
	lastOfs, offset := 0, 1
	if s.less(arr[base+hint], key) {
		maxOfs := length - hint
		for offset < maxOfs {
			if !s.less(arr[base+hint+offset], key) {
				break
			}
			lastOfs = offset
			offset = (offset << 1) + 1
		}
		if offset > maxOfs {
			offset = maxOfs
		}
		lastOfs += hint
		offset += hint
	} else {
		maxOfs := hint + 1
		for offset < maxOfs {
			if s.less(arr[base+hint-offset], key) {
				break
			}
			lastOfs = offset
			offset = (offset << 1) + 1
		}
		if offset > maxOfs {
			offset = maxOfs
		}
		lastOfs, offset = hint-offset, hint-lastOfs
	}
	lastOfs++
	for lastOfs < offset {
		m := lastOfs + ((offset - lastOfs) >> 1)
		if s.less(arr[base+m], key) {
			lastOfs = m + 1
		} else {
			offset = m
		}
	}
	return offset
}

// gallopRight is gallopLeft for the rightmost position: equal elements stay
// to the left of key.
func gallopRight[T any](s *timSort[T], arr []T, key T, base, length, hint int) int {
	lastOfs, offset := 0, 1
	if s.less(key, arr[base+hint]) {
		maxOfs := hint + 1
		for offset < maxOfs {
			if !s.less(key, arr[base+hint-offset]) {
				break
			}
			lastOfs = offset
			offset = (offset << 1) + 1
		}
		if offset > maxOfs {
			offset = maxOfs
		}
		lastOfs, offset = hint-offset, hint-lastOfs
	} else {
		maxOfs := length - hint
		for offset < maxOfs {
			if s.less(key, arr[base+hint+offset]) {
				break
			}
			lastOfs = offset
			offset = (offset << 1) + 1
		}
		if offset > maxOfs {
			offset = maxOfs
		}
		lastOfs += hint
		offset += hint
	}
	lastOfs++
	for lastOfs < offset {
		m := lastOfs + ((offset - lastOfs) >> 1)
		if s.less(key, arr[base+m]) {
			offset = m
		} else {
			lastOfs = m + 1
		}
	}
	return offset
}

func (s *timSort[T]) mergeLow(baseA, lengthA, baseB, lengthB int) {
	a := s.a
	temp := append([]T(nil), a[baseA:baseA+lengthA]...)
	dest, cursorTemp, cursorB := baseA, 0, baseB

	a[dest] = a[cursorB]
	dest++
	cursorB++

	copyB := func() {
		copy(a[dest:dest+lengthB], a[cursorB:cursorB+lengthB])
		a[dest+lengthB] = temp[cursorTemp]
	}
	succeed := func() {
		if lengthA > 0 {
			copy(a[dest:dest+lengthA], temp[cursorTemp:cursorTemp+lengthA])
		}
	}

	lengthB--
	if lengthB == 0 {
		succeed()
		return
	}
	if lengthA == 1 {
		copyB()
		return
	}

	minGallop := s.minGallop
	for {
		winsA, winsB := 0, 0
		for {
			if s.less(a[cursorB], temp[cursorTemp]) {
				a[dest] = a[cursorB]
				dest++
				cursorB++
				winsB++
				lengthB--
				winsA = 0
				if lengthB == 0 {
					succeed()
					return
				}
				if winsB >= minGallop {
					break
				}
			} else {
				a[dest] = temp[cursorTemp]
				dest++
				cursorTemp++
				winsA++
				lengthA--
				winsB = 0
				if lengthA == 1 {
					copyB()
					return
				}
				if winsA >= minGallop {
					break
				}
			}
		}

		minGallop++
		first := true
		for winsA >= minGallopWins || winsB >= minGallopWins || first {
			first = false
			minGallop--
			if minGallop < 1 {
				minGallop = 1
			}
			s.minGallop = minGallop

			winsA = gallopRight(s, temp, a[cursorB], cursorTemp, lengthA, 0)
			if winsA > 0 {
				copy(a[dest:dest+winsA], temp[cursorTemp:cursorTemp+winsA])
				dest += winsA
				cursorTemp += winsA
				lengthA -= winsA
				if lengthA == 1 {
					copyB()
					return
				}
				if lengthA == 0 {
					succeed()
					return
				}
			}
			a[dest] = a[cursorB]
			dest++
			cursorB++
			lengthB--
			if lengthB == 0 {
				succeed()
				return
			}

			winsB = gallopLeft(s, a, temp[cursorTemp], cursorB, lengthB, 0)
			if winsB > 0 {
				copy(a[dest:dest+winsB], a[cursorB:cursorB+winsB])
				dest += winsB
				cursorB += winsB
				lengthB -= winsB
				if lengthB == 0 {
					succeed()
					return
				}
			}
			a[dest] = temp[cursorTemp]
			dest++
			cursorTemp++
			lengthA--
			if lengthA == 1 {
				copyB()
				return
			}
		}
		minGallop++
		s.minGallop = minGallop
	}
}

func (s *timSort[T]) mergeHigh(baseA, lengthA, baseB, lengthB int) {
	a := s.a
	temp := append([]T(nil), a[baseB:baseB+lengthB]...)
	dest := baseB + lengthB - 1
	cursorTemp := lengthB - 1
	cursorA := baseA + lengthA - 1

	a[dest] = a[cursorA]
	dest--
	cursorA--

	copyA := func() {
		dest -= lengthA
		cursorA -= lengthA
		copy(a[dest+1:dest+1+lengthA], a[cursorA+1:cursorA+1+lengthA])
		a[dest] = temp[cursorTemp]
	}
	succeed := func() {
		if lengthB > 0 {
			copy(a[dest-(lengthB-1):dest+1], temp[:lengthB])
		}
	}

	lengthA--
	if lengthA == 0 {
		succeed()
		return
	}
	if lengthB == 1 {
		copyA()
		return
	}

	minGallop := s.minGallop
	for {
		winsA, winsB := 0, 0
		for {
			if s.less(temp[cursorTemp], a[cursorA]) {
				a[dest] = a[cursorA]
				dest--
				cursorA--
				winsA++
				lengthA--
				winsB = 0
				if lengthA == 0 {
					succeed()
					return
				}
				if winsA >= minGallop {
					break
				}
			} else {
				a[dest] = temp[cursorTemp]
				dest--
				cursorTemp--
				winsB++
				lengthB--
				winsA = 0
				if lengthB == 1 {
					copyA()
					return
				}
				if winsB >= minGallop {
					break
				}
			}
		}

		minGallop++
		first := true
		for winsA >= minGallopWins || winsB >= minGallopWins || first {
			first = false
			minGallop--
			if minGallop < 1 {
				minGallop = 1
			}
			s.minGallop = minGallop

			k := gallopRight(s, a, temp[cursorTemp], baseA, lengthA, lengthA-1)
			winsA = lengthA - k
			if winsA > 0 {
				dest -= winsA
				cursorA -= winsA
				copy(a[dest+1:dest+1+winsA], a[cursorA+1:cursorA+1+winsA])
				lengthA -= winsA
				if lengthA == 0 {
					succeed()
					return
				}
			}
			a[dest] = temp[cursorTemp]
			dest--
			cursorTemp--
			lengthB--
			if lengthB == 1 {
				copyA()
				return
			}

			k = gallopLeft(s, temp, a[cursorA], 0, lengthB, lengthB-1)
			winsB = lengthB - k
			if winsB > 0 {
				dest -= winsB
				cursorTemp -= winsB
				copy(a[dest+1:dest+1+winsB], temp[cursorTemp+1:cursorTemp+1+winsB])
				lengthB -= winsB
				if lengthB == 1 {
					copyA()
					return
				}
				if lengthB == 0 {
					succeed()
					return
				}
			}
			a[dest] = a[cursorA]
			dest--
			cursorA--
			lengthA--
			if lengthA == 0 {
				succeed()
				return
			}
		}
		minGallop++
		s.minGallop = minGallop
	}
}
