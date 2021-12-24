package routing

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
)

var ErrTargetNotFound = errors.New("Target not found")

type Target struct {
	Service               string `json:"service"`
	Port                  int    `json:"port"`
	Deployment            string `json:"deployment"`
	TargetPendingRequests int32  `json:"target"`
}

// NewTarget creates a new Target from the given parameters.
func NewTarget(
	svc string,
	port int,
	depl string,
	target int32,
) Target {
	return Target{
		Service:               svc,
		Port:                  port,
		Deployment:            depl,
		TargetPendingRequests: target,
	}
}

func (t *Target) ServiceURL() (*url.URL, error) {
	urlStr := fmt.Sprintf("http://%s:%d", t.Service, t.Port)
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	return u, nil

}

type TableReader interface {
	Lookup(string) (Target, error)
}
type Table struct {
	fmt.Stringer
	m map[string]Target
	l *sync.RWMutex
}

func NewTable() *Table {
	return &Table{
		m: make(map[string]Target),
		l: new(sync.RWMutex),
	}
}

func (t *Table) String() string {
	t.l.RLock()
	defer t.l.RUnlock()
	return fmt.Sprintf("%v", t.m)
}

func (t *Table) MarshalJSON() ([]byte, error) {
	t.l.RLock()
	defer t.l.RUnlock()
	var b bytes.Buffer
	err := json.NewEncoder(&b).Encode(t.m)
	if err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (t *Table) UnmarshalJSON(data []byte) error {
	t.l.Lock()
	defer t.l.Unlock()
	t.m = map[string]Target{}
	b := bytes.NewBuffer(data)
	return json.NewDecoder(b).Decode(&t.m)
}

func (t *Table) Lookup(host string) (Target, error) {
	t.l.RLock()
	defer t.l.RUnlock()
	k, err := t.lookup(host)
	if err != nil {
		return Target{}, err
	}
	return t.m[k], nil
}

// AddTarget registers target for host in the routing table t
// if it didn't already exist.
//
// returns a non-nil error if it did already exist
func (t *Table) AddTarget(
	host string,
	target Target,
) error {
	n := strings.Count(host, "*")
	if n > 0 && (!strings.HasPrefix(host, "*") || n > 1) {
		return fmt.Errorf("invalid wildcard for host %s", host)
	} else if n > 0 {
		host = "^" + strings.Replace(host, "*", "[a-zA-Z0-9-]+", 1) + "$"
	}

	t.l.Lock()
	defer t.l.Unlock()
	_, err := t.lookup(host)
	if err == ErrTargetNotFound {
		t.m[host] = target
		return nil
	}
	return fmt.Errorf(
		"host %s is already registered in the routing table",
		host,
	)
}

// RemoveTarget removes host, if it exists, and its corresponding Target entry in
// the routing table. If it does not exist, returns a non-nil error
func (t *Table) RemoveTarget(host string) error {
	t.l.Lock()
	defer t.l.Unlock()
	k, err := t.lookup(host)
	if err == ErrTargetNotFound {
		return fmt.Errorf("host %s did not exist in the routing table", host)
	}
	delete(t.m, k)
	return nil
}

// Replace replaces t's routing table with newTable's.
//
// This function is concurrency safe for t, but not for newTable.
// The caller must ensure that no other goroutine is writing to
// newTable at the time at which they call this function.
func (t *Table) Replace(newTable *Table) {
	t.l.Lock()
	defer t.l.Unlock()
	t.m = newTable.m
}

func (t *Table) lookup(host string) (string, error) {
	if _, ok := t.m[host]; ok {
		return host, nil
	}
	for k := range t.m {
		if matched, _ := regexp.MatchString(k, host); matched {
			return k, nil
		}
	}
	return "", ErrTargetNotFound
}
