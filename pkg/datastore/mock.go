// Code generated by MockGen. DO NOT EDIT.
// Source: pkg/datastore/datastore.go

// Package datastore is a generated GoMock package.
package datastore

import (
	context "context"
	gomock "github.com/golang/mock/gomock"
	reflect "reflect"
)

// MockDataStore is a mock of DataStore interface
type MockDataStore struct {
	ctrl     *gomock.Controller
	recorder *MockDataStoreMockRecorder
}

// MockDataStoreMockRecorder is the mock recorder for MockDataStore
type MockDataStoreMockRecorder struct {
	mock *MockDataStore
}

// NewMockDataStore creates a new mock instance
func NewMockDataStore(ctrl *gomock.Controller) *MockDataStore {
	mock := &MockDataStore{ctrl: ctrl}
	mock.recorder = &MockDataStoreMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockDataStore) EXPECT() *MockDataStoreMockRecorder {
	return m.recorder
}

// Find mocks base method
func (m *MockDataStore) Find(ctx context.Context, kind string, opts ListOptions) (Iterator, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Find", ctx, kind, opts)
	ret0, _ := ret[0].(Iterator)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Find indicates an expected call of Find
func (mr *MockDataStoreMockRecorder) Find(ctx, kind, opts interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Find", reflect.TypeOf((*MockDataStore)(nil).Find), ctx, kind, opts)
}

// Get mocks base method
func (m *MockDataStore) Get(ctx context.Context, kind, id string, entity interface{}) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Get", ctx, kind, id, entity)
	ret0, _ := ret[0].(error)
	return ret0
}

// Get indicates an expected call of Get
func (mr *MockDataStoreMockRecorder) Get(ctx, kind, id, entity interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Get", reflect.TypeOf((*MockDataStore)(nil).Get), ctx, kind, id, entity)
}

// Create mocks base method
func (m *MockDataStore) Create(ctx context.Context, kind, id string, entity interface{}) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Create", ctx, kind, id, entity)
	ret0, _ := ret[0].(error)
	return ret0
}

// Create indicates an expected call of Create
func (mr *MockDataStoreMockRecorder) Create(ctx, kind, id, entity interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Create", reflect.TypeOf((*MockDataStore)(nil).Create), ctx, kind, id, entity)
}

// Put mocks base method
func (m *MockDataStore) Put(ctx context.Context, kind, id string, entity interface{}) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Put", ctx, kind, id, entity)
	ret0, _ := ret[0].(error)
	return ret0
}

// Put indicates an expected call of Put
func (mr *MockDataStoreMockRecorder) Put(ctx, kind, id, entity interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Put", reflect.TypeOf((*MockDataStore)(nil).Put), ctx, kind, id, entity)
}

// Update mocks base method
func (m *MockDataStore) Update(ctx context.Context, kind, id string, factory Factory, updater Updater) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Update", ctx, kind, id, factory, updater)
	ret0, _ := ret[0].(error)
	return ret0
}

// Update indicates an expected call of Update
func (mr *MockDataStoreMockRecorder) Update(ctx, kind, id, factory, updater interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Update", reflect.TypeOf((*MockDataStore)(nil).Update), ctx, kind, id, factory, updater)
}

// Close mocks base method
func (m *MockDataStore) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

// Close indicates an expected call of Close
func (mr *MockDataStoreMockRecorder) Close() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockDataStore)(nil).Close))
}

// MockIterator is a mock of Iterator interface
type MockIterator struct {
	ctrl     *gomock.Controller
	recorder *MockIteratorMockRecorder
}

// MockIteratorMockRecorder is the mock recorder for MockIterator
type MockIteratorMockRecorder struct {
	mock *MockIterator
}

// NewMockIterator creates a new mock instance
func NewMockIterator(ctrl *gomock.Controller) *MockIterator {
	mock := &MockIterator{ctrl: ctrl}
	mock.recorder = &MockIteratorMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockIterator) EXPECT() *MockIteratorMockRecorder {
	return m.recorder
}

// Next mocks base method
func (m *MockIterator) Next(dst interface{}) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Next", dst)
	ret0, _ := ret[0].(error)
	return ret0
}

// Next indicates an expected call of Next
func (mr *MockIteratorMockRecorder) Next(dst interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Next", reflect.TypeOf((*MockIterator)(nil).Next), dst)
}

// Cursor mocks base method
func (m *MockIterator) Cursor() (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Cursor")
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Cursor indicates an expected call of Cursor
func (mr *MockIteratorMockRecorder) Cursor() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Cursor", reflect.TypeOf((*MockIterator)(nil).Cursor))
}
