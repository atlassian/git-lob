// Generated by: gen
// TypeWriter: gen
// Directive: +gen on main.string

// See http://clipperhouse.github.io/gen for documentation

// Sort implementation is a modification of http://golang.org/pkg/sort/#Sort
// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found at http://golang.org/LICENSE.

package main

import (
	"errors"
	"sort"
)

// Strings is a slice of type string. Use it where you would use []string.
type Strings []string

// All verifies that all elements of Strings return true for the passed func. See: http://clipperhouse.github.io/gen/#All
func (rcv Strings) All(fn func(string) bool) bool {
	for _, v := range rcv {
		if !fn(v) {
			return false
		}
	}
	return true
}

// Any verifies that one or more elements of Strings return true for the passed func. See: http://clipperhouse.github.io/gen/#Any
func (rcv Strings) Any(fn func(string) bool) bool {
	for _, v := range rcv {
		if fn(v) {
			return true
		}
	}
	return false
}

// Count gives the number elements of Strings that return true for the passed func. See: http://clipperhouse.github.io/gen/#Count
func (rcv Strings) Count(fn func(string) bool) (result int) {
	for _, v := range rcv {
		if fn(v) {
			result++
		}
	}
	return
}

// Distinct returns a new Strings slice whose elements are unique. See: http://clipperhouse.github.io/gen/#Distinct
func (rcv Strings) Distinct() (result Strings) {
	appended := make(map[string]bool)
	for _, v := range rcv {
		if !appended[v] {
			result = append(result, v)
			appended[v] = true
		}
	}
	return result
}

// DistinctBy returns a new Strings slice whose elements are unique, where equality is defined by a passed func. See: http://clipperhouse.github.io/gen/#DistinctBy
func (rcv Strings) DistinctBy(equal func(string, string) bool) (result Strings) {
	for _, v := range rcv {
		eq := func(_app string) bool {
			return equal(v, _app)
		}
		if !result.Any(eq) {
			result = append(result, v)
		}
	}
	return result
}

// Each iterates over Strings and executes the passed func against each element. See: http://clipperhouse.github.io/gen/#Each
func (rcv Strings) Each(fn func(string)) {
	for _, v := range rcv {
		fn(v)
	}
}

// First returns the first element that returns true for the passed func. Returns error if no elements return true. See: http://clipperhouse.github.io/gen/#First
func (rcv Strings) First(fn func(string) bool) (result string, err error) {
	for _, v := range rcv {
		if fn(v) {
			result = v
			return
		}
	}
	err = errors.New("no Strings elements return true for passed func")
	return
}

// IsSorted reports whether Strings is sorted. See: http://clipperhouse.github.io/gen/#Sort
func (rcv Strings) IsSorted() bool {
	return sort.IsSorted(rcv)
}

// IsSortedBy reports whether an instance of Strings is sorted, using the pass func to define ‘less’. See: http://clipperhouse.github.io/gen/#SortBy
func (rcv Strings) IsSortedBy(less func(string, string) bool) bool {
	n := len(rcv)
	for i := n - 1; i > 0; i-- {
		if less(rcv[i], rcv[i-1]) {
			return false
		}
	}
	return true
}

// IsSortedDesc reports whether an instance of Strings is sorted in descending order, using the pass func to define ‘less’. See: http://clipperhouse.github.io/gen/#SortBy
func (rcv Strings) IsSortedByDesc(less func(string, string) bool) bool {
	greater := func(a, b string) bool {
		return less(b, a)
	}
	return rcv.IsSortedBy(greater)
}

// IsSortedDesc reports whether Strings is reverse-sorted. See: http://clipperhouse.github.io/gen/#Sort
func (rcv Strings) IsSortedDesc() bool {
	return sort.IsSorted(sort.Reverse(rcv))
}

// Max returns the maximum value of Strings. In the case of multiple items being equally maximal, the first such element is returned. Returns error if no elements. See: http://clipperhouse.github.io/gen/#Max
func (rcv Strings) Max() (result string, err error) {
	l := len(rcv)
	if l == 0 {
		err = errors.New("cannot determine the Max of an empty slice")
		return
	}
	result = rcv[0]
	for _, v := range rcv {
		if v > result {
			result = v
		}
	}
	return
}

// MaxBy returns an element of Strings containing the maximum value, when compared to other elements using a passed func defining ‘less’. In the case of multiple items being equally maximal, the last such element is returned. Returns error if no elements. See: http://clipperhouse.github.io/gen/#MaxBy
func (rcv Strings) MaxBy(less func(string, string) bool) (result string, err error) {
	l := len(rcv)
	if l == 0 {
		err = errors.New("cannot determine the MaxBy of an empty slice")
		return
	}
	m := 0
	for i := 1; i < l; i++ {
		if rcv[i] != rcv[m] && !less(rcv[i], rcv[m]) {
			m = i
		}
	}
	result = rcv[m]
	return
}

// Min returns the minimum value of Strings. In the case of multiple items being equally minimal, the first such element is returned. Returns error if no elements. See: http://clipperhouse.github.io/gen/#Min
func (rcv Strings) Min() (result string, err error) {
	l := len(rcv)
	if l == 0 {
		err = errors.New("cannot determine the Min of an empty slice")
		return
	}
	result = rcv[0]
	for _, v := range rcv {
		if v < result {
			result = v
		}
	}
	return
}

// MinBy returns an element of Strings containing the minimum value, when compared to other elements using a passed func defining ‘less’. In the case of multiple items being equally minimal, the first such element is returned. Returns error if no elements. See: http://clipperhouse.github.io/gen/#MinBy
func (rcv Strings) MinBy(less func(string, string) bool) (result string, err error) {
	l := len(rcv)
	if l == 0 {
		err = errors.New("cannot determine the Min of an empty slice")
		return
	}
	m := 0
	for i := 1; i < l; i++ {
		if less(rcv[i], rcv[m]) {
			m = i
		}
	}
	result = rcv[m]
	return
}

// Single returns exactly one element of Strings that returns true for the passed func. Returns error if no or multiple elements return true. See: http://clipperhouse.github.io/gen/#Single
func (rcv Strings) Single(fn func(string) bool) (result string, err error) {
	var candidate string
	found := false
	for _, v := range rcv {
		if fn(v) {
			if found {
				err = errors.New("multiple Strings elements return true for passed func")
				return
			}
			candidate = v
			found = true
		}
	}
	if found {
		result = candidate
	} else {
		err = errors.New("no Strings elements return true for passed func")
	}
	return
}

// Sort returns a new ordered Strings slice. See: http://clipperhouse.github.io/gen/#Sort
func (rcv Strings) Sort() Strings {
	result := make(Strings, len(rcv))
	copy(result, rcv)
	sort.Sort(result)
	return result
}

// SortBy returns a new ordered Strings slice, determined by a func defining ‘less’. See: http://clipperhouse.github.io/gen/#SortBy
func (rcv Strings) SortBy(less func(string, string) bool) Strings {
	result := make(Strings, len(rcv))
	copy(result, rcv)
	// Switch to heapsort if depth of 2*ceil(lg(n+1)) is reached.
	n := len(result)
	maxDepth := 0
	for i := n; i > 0; i >>= 1 {
		maxDepth++
	}
	maxDepth *= 2
	quickSortStrings(result, less, 0, n, maxDepth)
	return result
}

// SortByDesc returns a new, descending-ordered Strings slice, determined by a func defining ‘less’. See: http://clipperhouse.github.io/gen/#SortBy
func (rcv Strings) SortByDesc(less func(string, string) bool) Strings {
	greater := func(a, b string) bool {
		return less(b, a)
	}
	return rcv.SortBy(greater)
}

// SortDesc returns a new reverse-ordered Strings slice. See: http://clipperhouse.github.io/gen/#Sort
func (rcv Strings) SortDesc() Strings {
	result := make(Strings, len(rcv))
	copy(result, rcv)
	sort.Sort(sort.Reverse(result))
	return result
}

// Where returns a new Strings slice whose elements return true for func. See: http://clipperhouse.github.io/gen/#Where
func (rcv Strings) Where(fn func(string) bool) (result Strings) {
	for _, v := range rcv {
		if fn(v) {
			result = append(result, v)
		}
	}
	return result
}

func (rcv Strings) Len() int {
	return len(rcv)
}
func (rcv Strings) Less(i, j int) bool {
	return rcv[i] < rcv[j]
}
func (rcv Strings) Swap(i, j int) {
	rcv[i], rcv[j] = rcv[j], rcv[i]
}

// Sort implementation based on http://golang.org/pkg/sort/#Sort, see top of this file

func swapStrings(rcv Strings, a, b int) {
	rcv[a], rcv[b] = rcv[b], rcv[a]
}

// Insertion sort
func insertionSortStrings(rcv Strings, less func(string, string) bool, a, b int) {
	for i := a + 1; i < b; i++ {
		for j := i; j > a && less(rcv[j], rcv[j-1]); j-- {
			swapStrings(rcv, j, j-1)
		}
	}
}

// siftDown implements the heap property on rcv[lo, hi).
// first is an offset into the array where the root of the heap lies.
func siftDownStrings(rcv Strings, less func(string, string) bool, lo, hi, first int) {
	root := lo
	for {
		child := 2*root + 1
		if child >= hi {
			break
		}
		if child+1 < hi && less(rcv[first+child], rcv[first+child+1]) {
			child++
		}
		if !less(rcv[first+root], rcv[first+child]) {
			return
		}
		swapStrings(rcv, first+root, first+child)
		root = child
	}
}

func heapSortStrings(rcv Strings, less func(string, string) bool, a, b int) {
	first := a
	lo := 0
	hi := b - a

	// Build heap with greatest element at top.
	for i := (hi - 1) / 2; i >= 0; i-- {
		siftDownStrings(rcv, less, i, hi, first)
	}

	// Pop elements, largest first, into end of rcv.
	for i := hi - 1; i >= 0; i-- {
		swapStrings(rcv, first, first+i)
		siftDownStrings(rcv, less, lo, i, first)
	}
}

// Quicksort, following Bentley and McIlroy,
// Engineering a Sort Function, SP&E November 1993.

// medianOfThree moves the median of the three values rcv[a], rcv[b], rcv[c] into rcv[a].
func medianOfThreeStrings(rcv Strings, less func(string, string) bool, a, b, c int) {
	m0 := b
	m1 := a
	m2 := c
	// bubble sort on 3 elements
	if less(rcv[m1], rcv[m0]) {
		swapStrings(rcv, m1, m0)
	}
	if less(rcv[m2], rcv[m1]) {
		swapStrings(rcv, m2, m1)
	}
	if less(rcv[m1], rcv[m0]) {
		swapStrings(rcv, m1, m0)
	}
	// now rcv[m0] <= rcv[m1] <= rcv[m2]
}

func swapRangeStrings(rcv Strings, a, b, n int) {
	for i := 0; i < n; i++ {
		swapStrings(rcv, a+i, b+i)
	}
}

func doPivotStrings(rcv Strings, less func(string, string) bool, lo, hi int) (midlo, midhi int) {
	m := lo + (hi-lo)/2 // Written like this to avoid integer overflow.
	if hi-lo > 40 {
		// Tukey's Ninther, median of three medians of three.
		s := (hi - lo) / 8
		medianOfThreeStrings(rcv, less, lo, lo+s, lo+2*s)
		medianOfThreeStrings(rcv, less, m, m-s, m+s)
		medianOfThreeStrings(rcv, less, hi-1, hi-1-s, hi-1-2*s)
	}
	medianOfThreeStrings(rcv, less, lo, m, hi-1)

	// Invariants are:
	//	rcv[lo] = pivot (set up by ChoosePivot)
	//	rcv[lo <= i < a] = pivot
	//	rcv[a <= i < b] < pivot
	//	rcv[b <= i < c] is unexamined
	//	rcv[c <= i < d] > pivot
	//	rcv[d <= i < hi] = pivot
	//
	// Once b meets c, can swap the "= pivot" sections
	// into the middle of the slice.
	pivot := lo
	a, b, c, d := lo+1, lo+1, hi, hi
	for {
		for b < c {
			if less(rcv[b], rcv[pivot]) { // rcv[b] < pivot
				b++
			} else if !less(rcv[pivot], rcv[b]) { // rcv[b] = pivot
				swapStrings(rcv, a, b)
				a++
				b++
			} else {
				break
			}
		}
		for b < c {
			if less(rcv[pivot], rcv[c-1]) { // rcv[c-1] > pivot
				c--
			} else if !less(rcv[c-1], rcv[pivot]) { // rcv[c-1] = pivot
				swapStrings(rcv, c-1, d-1)
				c--
				d--
			} else {
				break
			}
		}
		if b >= c {
			break
		}
		// rcv[b] > pivot; rcv[c-1] < pivot
		swapStrings(rcv, b, c-1)
		b++
		c--
	}

	min := func(a, b int) int {
		if a < b {
			return a
		}
		return b
	}

	n := min(b-a, a-lo)
	swapRangeStrings(rcv, lo, b-n, n)

	n = min(hi-d, d-c)
	swapRangeStrings(rcv, c, hi-n, n)

	return lo + b - a, hi - (d - c)
}

func quickSortStrings(rcv Strings, less func(string, string) bool, a, b, maxDepth int) {
	for b-a > 7 {
		if maxDepth == 0 {
			heapSortStrings(rcv, less, a, b)
			return
		}
		maxDepth--
		mlo, mhi := doPivotStrings(rcv, less, a, b)
		// Avoiding recursion on the larger subproblem guarantees
		// a stack depth of at most lg(b-a).
		if mlo-a < b-mhi {
			quickSortStrings(rcv, less, a, mlo, maxDepth)
			a = mhi // i.e., quickSortStrings(rcv, mhi, b)
		} else {
			quickSortStrings(rcv, less, mhi, b, maxDepth)
			b = mlo // i.e., quickSortStrings(rcv, a, mlo)
		}
	}
	if b-a > 1 {
		insertionSortStrings(rcv, less, a, b)
	}
}