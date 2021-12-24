package queue

import (
	"fmt"
	"regexp"
	"sync"
	"time"
)

var _ Counter = &FakeCounter{}

type HostAndCount struct {
	Host  string
	Count int
}
type FakeCounter struct {
	mapMut        *sync.RWMutex
	RetMap        map[string]int
	ResizedCh     chan HostAndCount
	ResizeTimeout time.Duration
}

func NewFakeCounter() *FakeCounter {
	return &FakeCounter{
		mapMut:        new(sync.RWMutex),
		RetMap:        map[string]int{},
		ResizedCh:     make(chan HostAndCount),
		ResizeTimeout: 1 * time.Second,
	}
}

func (f *FakeCounter) Resize(host string, i int) error {
	f.mapMut.Lock()
	if k, err := f.Lookup(host); err == ErrTargetNotFound {
		f.RetMap[host] += i
	} else {
		f.RetMap[k] += i
	}

	f.mapMut.Unlock()
	select {
	case f.ResizedCh <- HostAndCount{Host: host, Count: i}:
	case <-time.After(f.ResizeTimeout):
		return fmt.Errorf(
			"FakeCounter.Resize timeout after %s",
			f.ResizeTimeout,
		)
	}
	return nil
}

func (f *FakeCounter) Ensure(host string) {
	f.mapMut.Lock()
	defer f.mapMut.Unlock()
	k, err := f.Lookup(host)
	if err == ErrTargetNotFound {
		f.RetMap[host] = 0
		return
	}
	f.RetMap[k] = 0
}

func (f *FakeCounter) Remove(host string) bool {
	f.mapMut.Lock()
	defer f.mapMut.Unlock()
	k, ok := f.Lookup(host)
	delete(f.RetMap, k)
	return ok == nil
}

func (f *FakeCounter) Current() (*Counts, error) {
	ret := NewCounts()
	f.mapMut.RLock()
	defer f.mapMut.RUnlock()
	retMap := f.RetMap
	if len(retMap) == 0 {
		retMap["sample.com"] = 0
	}
	ret.Counts = retMap
	return ret, nil
}

var _ CountReader = &FakeCountReader{}

type FakeCountReader struct {
	current int
	err     error
}

func (f *FakeCountReader) Current() (*Counts, error) {
	ret := NewCounts()
	ret.Counts = map[string]int{
		"sample.com": f.current,
	}
	return ret, f.err
}

func (f *FakeCounter) Lookup(host string) (string, error) {
	if _, ok := f.RetMap[host]; ok {
		return host, nil
	}
	for k := range f.RetMap {
		if matched, _ := regexp.MatchString(k, host); matched {
			return k, nil
		}
	}
	return "", ErrTargetNotFound
}
