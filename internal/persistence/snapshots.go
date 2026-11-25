// Package persistence translates shared application snapshots into database records.
package persistence

import (
	"encoding/json"
	"errors"

	"github.com/zeroy1024/Burrow/internal/profile"
	"github.com/zeroy1024/Burrow/internal/store"
	"github.com/zeroy1024/Burrow/internal/transfer"
	"github.com/zeroy1024/Burrow/internal/workspace"
)

type endpointEncoder struct{ values map[string]store.Endpoint }

func (e *endpointEncoder) add(value *profile.Endpoint) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	id := store.EndpointID(data)
	if e.values == nil {
		e.values = map[string]store.Endpoint{}
	}
	e.values[id] = store.Endpoint{ID: id, KeyID: value.KeyID, Value: data}
	*value = profile.Endpoint{}
	return id, nil
}
func (e *endpointEncoder) endpoints() []store.Endpoint {
	result := make([]store.Endpoint, 0, len(e.values))
	for _, v := range e.values {
		result = append(result, v)
	}
	return result
}

type endpointDecoder map[string]store.Endpoint

func newEndpointDecoder(values []store.Endpoint) (endpointDecoder, error) {
	d := endpointDecoder{}
	for _, v := range values {
		if v.ID != store.EndpointID(v.Value) {
			return nil, errors.New("immutable endpoint digest mismatch")
		}
		d[v.ID] = v
	}
	return d, nil
}
func (d endpointDecoder) get(id string, value *profile.Endpoint) error {
	data, exists := d[id]
	if !exists {
		return errors.New("missing immutable endpoint " + id)
	}
	if err := json.Unmarshal(data.Value, value); err != nil {
		return err
	}
	if value.KeyID != data.KeyID {
		return errors.New("snapshot key reference mismatch")
	}
	return nil
}

func visitWorkspace(w *workspace.Snapshot, visit func(*profile.Endpoint, *string) error) error {
	for i := range w.Connections {
		v := &w.Connections[i]
		if err := visit(&v.Endpoint, &v.EndpointRef); err != nil {
			return err
		}
	}
	var walk func(*workspace.Layout[workspace.Session]) error
	walk = func(n *workspace.Layout[workspace.Session]) error {
		if n == nil {
			return nil
		}
		if n.Session != nil {
			return visit(&n.Session.Endpoint, &n.Session.EndpointRef)
		}
		if err := walk(n.First); err != nil {
			return err
		}
		return walk(n.Second)
	}
	for i := range w.Tabs {
		if err := walk(w.Tabs[i].Root); err != nil {
			return err
		}
	}
	for i := range w.FileTabs {
		v := &w.FileTabs[i].Session
		if err := visit(&v.Endpoint, &v.EndpointRef); err != nil {
			return err
		}
	}
	return nil
}

func EncodeWorkspace(value *workspace.Snapshot) (store.Workspace, error) {
	// Copy before replacing full endpoints with database references.
	data, err := json.Marshal(value)
	if err != nil {
		return store.Workspace{}, err
	}
	var w workspace.Snapshot
	if err := json.Unmarshal(data, &w); err != nil {
		return store.Workspace{}, err
	}
	e := endpointEncoder{}
	err = visitWorkspace(&w, func(v *profile.Endpoint, id *string) error { var err error; *id, err = e.add(v); return err })
	if err != nil {
		return store.Workspace{}, err
	}
	data, err = json.Marshal(w)
	return store.Workspace{Layout: data, Endpoints: e.endpoints()}, err
}

func DecodeWorkspace(value store.Workspace) (*workspace.Snapshot, error) {
	d, err := newEndpointDecoder(value.Endpoints)
	if err != nil {
		return nil, err
	}
	var w workspace.Snapshot
	if err := json.Unmarshal(value.Layout, &w); err != nil {
		return nil, err
	}
	err = visitWorkspace(&w, func(v *profile.Endpoint, id *string) error { return d.get(*id, v) })
	return &w, err
}

func EncodeTransfers(saved *transfer.Snapshot) (store.Transfers, error) {
	q := store.Transfers{NextID: saved.NextID, NextBatchID: saved.NextBatchID}
	e := endpointEncoder{}
	for _, b := range saved.Batches {
		q.Batches = append(q.Batches, store.TransferBatch{ID: b.ID, Policy: b.Policy})
	}
	for _, v := range saved.Sessions {
		var err error
		v.Session.EndpointRef, err = e.add(&v.Session.Endpoint)
		if err != nil {
			return q, err
		}
		data, err := json.Marshal(v)
		if err != nil {
			return q, err
		}
		q.Sessions = append(q.Sessions, store.TransferValue{ID: v.ID, Value: data})
	}
	for _, v := range saved.Connections {
		var err error
		v.EndpointRef, err = e.add(&v.Endpoint)
		if err != nil {
			return q, err
		}
		for _, step := range v.Steps {
			id, err := e.add(&step)
			if err != nil {
				return q, err
			}
			v.StepRefs = append(v.StepRefs, id)
		}
		v.Steps = nil
		data, err := json.Marshal(v)
		if err != nil {
			return q, err
		}
		q.Connections = append(q.Connections, store.TransferValue{ID: v.ID, Value: data})
	}
	for _, v := range saved.Tasks {
		data, err := json.Marshal(v)
		if err != nil {
			return q, err
		}
		q.Tasks = append(q.Tasks, store.TransferTask{ID: v.ID, BatchID: v.Batch, SessionID: v.Session, Value: data})
	}
	q.Endpoints = e.endpoints()
	return q, nil
}

func DecodeTransfers(q store.Transfers) (*transfer.Snapshot, error) {
	w := &transfer.Snapshot{Version: 4, NextID: q.NextID, NextBatchID: q.NextBatchID}
	d, err := newEndpointDecoder(q.Endpoints)
	if err != nil {
		return nil, err
	}
	for _, b := range q.Batches {
		w.Batches = append(w.Batches, transfer.SavedBatch{ID: b.ID, Policy: b.Policy})
	}
	for _, v := range q.Sessions {
		var s transfer.SavedSession
		if err := json.Unmarshal(v.Value, &s); err != nil {
			return nil, err
		}
		if s.ID != v.ID {
			return nil, errors.New("transfer session identity mismatch")
		}
		if err := d.get(s.Session.EndpointRef, &s.Session.Endpoint); err != nil {
			return nil, err
		}
		w.Sessions = append(w.Sessions, s)
	}
	for _, v := range q.Connections {
		var c transfer.SavedConnection
		if err := json.Unmarshal(v.Value, &c); err != nil {
			return nil, err
		}
		if c.ID != v.ID {
			return nil, errors.New("connection identity mismatch")
		}
		if err := d.get(c.EndpointRef, &c.Endpoint); err != nil {
			return nil, err
		}
		for _, id := range c.StepRefs {
			var s profile.Endpoint
			if err := d.get(id, &s); err != nil {
				return nil, err
			}
			c.Steps = append(c.Steps, s)
		}
		w.Connections = append(w.Connections, c)
	}
	for _, v := range q.Tasks {
		var t transfer.SavedTask
		if err := json.Unmarshal(v.Value, &t); err != nil {
			return nil, err
		}
		if t.ID != v.ID || t.Batch != v.BatchID || t.Session != v.SessionID {
			return nil, errors.New("transfer task identity mismatch")
		}
		w.Tasks = append(w.Tasks, t)
	}
	return w, nil
}
