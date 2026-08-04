package wsserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"sync"

	"github.com/jellydator/xync"
	"github.com/oxynote/wetsocks/internal/ctxutil"
	"github.com/oxynote/wetsocks/wsutil"
)

const (
	_descriptorSub   = "sub"
	_descriptorUnsub = "unsub"
	_descriptorPub   = "pub"
	_descriptorDrop  = "drop"
)

// Binder is an interface that is used to bind a specific topic to
// an event emitter.
type Binder interface {
	// Bind should pass the provided topic to an event emitter.
	Bind(tpc Topic)
}

// BinderFunc is an adapter allowing the use of ordinary functions
// as topic binders.
type BinderFunc func(Topic)

// Bind uses the receiver function to bind the topic to an event emitter.
func (b BinderFunc) Bind(tpc Topic) {
	b(tpc)
}

// Router routes websocket subscription requests to appropriate topics.
type Router struct {
	mu     sync.RWMutex
	topics map[string]*topic
}

// NewRouter creates a new instance topic router.
func NewRouter() *Router {
	return &Router{
		topics: make(map[string]*topic),
	}
}

// Bind registers a new topic and passes its control instance to the
// provided binder.
// The middlewares are called with each connection's context and built topic
// to determine whether that connection should be used when publishing events.
func (r *Router) Bind(
	rawTopic string,
	binder Binder,
	middlewares ...func(context.Context, string) bool,
) {
	r.mu.Lock()
	defer r.mu.Unlock()

	tpc, err := wsutil.ParseTopic(rawTopic)
	if err != nil {
		panic(fmt.Sprintf("wsserver: %s", err))
	}

	if binder == nil {
		panic("wsserver: invalid binder")
	}

	if _, ok := r.topics[rawTopic]; ok {
		panic(fmt.Sprintf("wsserver: multiple registrations for %s", rawTopic))
	}

	v := newTopic(tpc, middlewares...)
	r.topics[rawTopic] = v

	binder.Bind(v)
}

// BindFunc registers a new topic and passes its control instance to the
// provided binder function.
// The middlewares are called with each connection's context and built topic
// to determine whether that connection should be used when publishing events.
func (r *Router) BindFunc(
	rawTopic string,
	binderFn BinderFunc,
	middlewares ...func(context.Context, string) bool,
) {
	r.Bind(rawTopic, binderFn, middlewares...)
}

// Close closes all topic-related processes.
func (r *Router) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, tpc := range r.topics {
		tpc.supv.CloseAndWait()
	}
}

// findTopic finds a topic by the provided topic pattern and returns
// it alongside all topic string parameters.
// The returned parameters' map is nil only if the topic is nil.
// Not concurrently safe.
func (r *Router) findTopic(tpc wsutil.Topic) (*topic, map[string]string) {
Outer:
	for _, v := range r.topics {
		if v.pattern.topic.Operation != tpc.Operation {
			continue
		}

		if len(v.pattern.path) != len(tpc.Path) {
			continue
		}

		params := make(map[string]string)

		for i, pElem := range v.pattern.path {
			if pElem.placeholder {
				params[pElem.value] = tpc.Path[i]
				continue
			}

			if pElem.value != tpc.Path[i] {
				continue Outer
			}
		}

		return v, params
	}

	return nil, nil
}

// addTopicSub finds a topic by the provided topic pattern and adds the
// connection to its list.
func (r *Router) addTopicSub(tpc wsutil.Topic, sub *conn) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	v, params := r.findTopic(tpc)
	if v == nil {
		return errors.New("invalid topic")
	}

	v.addSub(sub, params)

	return nil
}

// removeTopicSub finds a topic by the provided topic pattern and removes
// the connection from its list.
func (r *Router) removeTopicSub(tpc wsutil.Topic, sub *conn) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	v, params := r.findTopic(tpc)
	if v == nil {
		return
	}

	v.removeSub(sub, params)
}

// removeConn iterates over all the router's topics and removes the
// provided connection from all of them.
func (r *Router) removeConn(c *conn) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, v := range r.topics {
		v.removeConn(c)
	}
}

// Topic is an interface that is used to control the subscription and event
// flow.
//
//go:generate ../scripts/codegen/mock Topic
type Topic interface {
	// PublishMany should send a new event with the provided payload
	// to all selected connections.
	// The optional func parameter should be called with each
	// connection's context and built topic to determine whether that
	// connection should be selected.
	PublishMany(
		ctx context.Context,
		payload any,
		filter func(context.Context, string) bool,
	)

	// PublishOne should send a new event with the provided payload
	// to the connection that is associated with the provided context.
	PublishOne(
		ctx context.Context,
		payload any,
	)

	// DropMany should remove the selected connections from the
	// subscribers list.
	// The reason string should be optional.
	// The optional func parameter should be called with each
	// connection's context and built topic to determine whether that
	// connection should be selected.
	// NOTE: if the func parameter is nil or not restrictive enough,
	// all subscribers may be removed.
	DropMany(
		ctx context.Context,
		reason string,
		filter func(context.Context, string) bool,
	)

	// DropOne should remove the connection that is associated with
	// the provided context from the subscribers list.
	// The reason string should be optional.
	DropOne(ctx context.Context, reason string)

	// OnSub should set the provided function to be executed
	// when a new subscription is added to the topic's subscribers list.
	// The context passed into the provided function should include data
	// of the new connection.
	OnSub(fn func(context.Context))

	// OnUnsub should set the provided function to be executed
	// when a subscription is removed from the topic's subscribers list.
	// The context passed into the provided function should include data
	// of the removed connection.
	OnUnsub(fn func(context.Context))

	// OnFirstSub should set the provided function to be executed
	// when the first subscription is added to the topic's
	// subscribers list.
	OnFirstSub(fn func(context.Context))

	// OnLastSub should set the provided function to be executed
	// when the last subscription is removed to the topic's
	// subscribers list.
	OnLastUnsub(fn func(context.Context))
}

// topic contains a list of subscribers and other auxiliary data
// that is used to improve the event flow.
type topic struct {
	pattern     pattern
	middlewares []func(context.Context, string) bool
	supv        *xync.Supervisor

	// subs is map of subscriber connections.
	// Each connection is associated with a slice of maps of topic
	// parameters each of which should be added to a new context
	// that extends the connection's context before each
	// middleware or sub/unsub callback call. Although it
	// would be easier to extend the connection's
	// context once and re-add it to the connection itself (e.g., on
	// successful subscription), the connection may subscribe
	// to other topics too and different topics may have their
	// own parameters that shouldn't be shared among other
	// topics. Furthermore, the connection's context
	// may be updated as well, which means it cannot
	// be extended and saved locally as this map's value because
	// after a while it would become outdated.
	subs   map[*conn][]map[string]string
	subsMu sync.RWMutex

	events struct {
		sub struct {
			mu  sync.RWMutex
			fns []func(context.Context)
		}
		unsub struct {
			mu  sync.RWMutex
			fns []func(context.Context)
		}
		firstSub struct {
			mu  sync.RWMutex
			fns []func(context.Context)
		}
		lastUnsub struct {
			mu  sync.RWMutex
			fns []func(context.Context)
		}
	}
}

// newTopic creates a new topic instance.
func newTopic(tpc wsutil.Topic, middlewares ...func(context.Context, string) bool) *topic {
	return &topic{
		pattern:     newPattern(tpc),
		middlewares: middlewares,
		supv:        xync.NewSupervisor(),
		subs:        make(map[*conn][]map[string]string),
	}
}

// addSub adds the provided connection to the subscribers list.
func (t *topic) addSub(sub *conn, params map[string]string) {
	t.subsMu.Lock()
	defer t.subsMu.Unlock()

	pp, ok := t.subs[sub]
	if ok {
		if !t.pattern.hasPlaceholders {
			return
		}

		// the same connection may subscribe to the same topic
		// but with different topic parameters
		for _, params1 := range pp {
			if !areTopicParamsEqual(params, params1) {
				continue
			}

			return
		}
	}

	if len(t.subs) == 0 {
		t.execFirstSubFns()
	}

	t.subs[sub] = append(pp, params)
	t.execSubFns(NewTopicParamsContext(sub.safeContext(), params))
}

// removeSub removes the provided connection from the subscribers list.
func (t *topic) removeSub(sub *conn, params map[string]string) {
	t.subsMu.Lock()
	defer t.subsMu.Unlock()

	pp, ok := t.subs[sub]
	if !ok {
		return
	}

	if t.pattern.hasPlaceholders {
		for i, params1 := range pp {
			if !areTopicParamsEqual(params, params1) {
				continue
			}

			// delete params without preserving order
			pp[i] = pp[len(pp)-1]
			pp[len(pp)-1] = nil
			pp = pp[:len(pp)-1]

			break
		}

		if len(pp) == 0 {
			delete(t.subs, sub)
		} else {
			t.subs[sub] = pp
		}
	} else {
		delete(t.subs, sub)
	}

	t.execUnsubFns(NewTopicParamsContext(sub.safeContext(), params))

	if len(t.subs) == 0 {
		t.execLastUnsubFns()
	}
}

// removeConn removes a connection from the subscribers list.
func (t *topic) removeConn(c *conn) {
	t.subsMu.Lock()
	defer t.subsMu.Unlock()

	pp, ok := t.subs[c]
	if !ok {
		return
	}

	for _, params := range pp {
		t.execUnsubFns(NewTopicParamsContext(c.safeContext(), params))
	}

	delete(t.subs, c)

	if len(t.subs) == 0 {
		t.execLastUnsubFns()
	}
}

// publish sends the payload to the provided subscribers.
func (t *topic) publish(
	pubCtx context.Context,
	ptrn pattern,
	subs map[*conn][]map[string]string,
	data map[string]any,
	middlewares ...func(context.Context, string) bool,
) {
	type event struct {
		data []byte
		err  error
	}

	var (
		wg        sync.WaitGroup
		presetsMu sync.Mutex
	)

	presets := make(map[string]event)

	fn := func(rawTopic string) func() ([]byte, error) {
		// this func is needed for two reasons:
		// - to delegate json error handling to conn publish func
		// - to ensure that we encode data only once per topic
		return func() ([]byte, error) {
			presetsMu.Lock()
			defer presetsMu.Unlock()

			preset, ok := presets[rawTopic]

			if !ok {
				topicData := make(map[string]any, len(data))
				maps.Copy(topicData, data)

				topicData["topic"] = rawTopic

				preset.data, preset.err = json.Marshal(topicData)
				presets[rawTopic] = preset
			}

			return preset.data, preset.err
		}
	}

	for sub, pp := range subs {
		wg.Add(len(pp))

		for _, params := range pp {
			subCtx := NewTopicParamsContext(sub.safeContext(), params)

			go func() {
				defer wg.Done()

				rawTopic := ptrn.format(params)

				for _, mfn := range middlewares {
					if !mfn(subCtx, rawTopic) {
						return
					}
				}

				sub.publish(pubCtx, fn(rawTopic))
			}()
		}
	}

	wg.Wait()
}

// PublishMany sends a new event with the provided payload
// to all selected connections.
// The func parameter is called with each connection's context and built
// topic to determine whether that connection should be selected.
func (t *topic) PublishMany(
	pubCtx context.Context,
	payload any,
	filter func(context.Context, string) bool,
) {
	t.subsMu.RLock()
	defer t.subsMu.RUnlock()

	middlewares := t.middlewares
	if filter != nil {
		middlewares = append(middlewares, filter)
	}

	ptrn := t.pattern
	ptrn.topic.Descriptor = _descriptorPub
	data := map[string]any{
		"payload": payload,
	}

	t.publish(pubCtx, ptrn, t.subs, data, middlewares...)
}

// PublishOne sends a new event with the provided payload
// to the connection that is associated with the provided context.
func (t *topic) PublishOne(
	ctx context.Context,
	payload any,
) {
	id := ctx.Value(_wsCtxID)
	if id == nil {
		return
	}

	t.PublishMany(ctx, payload, func(ctx context.Context, _ string) bool {
		return id == ctx.Value(_wsCtxID)
	})
}

// DropMany removes the selected connections from the subscribers
// list.
// The reason string is optional.
// The optional func parameter is called with each
// connection's context and built topic to determine whether that connection
// should be selected.
// NOTE: if the func parameter is nil or not restrictive enough,
// all subscribers may be removed.
func (t *topic) DropMany(
	dropCtx context.Context,
	reason string,
	filter func(context.Context, string) bool,
) {
	t.subsMu.Lock()
	defer t.subsMu.Unlock()

	droppedSubs := make(map[*conn][]map[string]string)

	for sub, pp := range t.subs {
		var (
			dropped  []map[string]string
			retained []map[string]string
		)

		for _, params := range pp {
			subCtx := NewTopicParamsContext(sub.safeContext(), params)

			rawTopic := t.pattern.format(params)

			if filter == nil || filter(subCtx, rawTopic) {
				dropped = append(dropped, params)

				t.execUnsubFns(subCtx)

				continue
			}

			retained = append(retained, params)
		}

		if len(dropped) > 0 {
			droppedSubs[sub] = dropped
		}

		if len(retained) == 0 {
			delete(t.subs, sub)
			continue
		}

		t.subs[sub] = retained
	}

	if len(t.subs) == 0 && len(droppedSubs) > 0 {
		t.execLastUnsubFns()
	}

	ptrn := t.pattern
	ptrn.topic.Descriptor = _descriptorDrop
	data := make(map[string]any)

	if reason != "" {
		data["reason"] = reason
	}

	t.publish(dropCtx, ptrn, droppedSubs, data)
}

// DropOne removes the connection that is associated with
// the provided context from the subscribers list.
// The reason string should be optional.
func (t *topic) DropOne(ctx context.Context, reason string) {
	id := ctx.Value(_wsCtxID)
	if id == nil {
		return
	}

	t.DropMany(ctx, reason, func(ctx context.Context, _ string) bool {
		return id == ctx.Value(_wsCtxID)
	})
}

// OnSub sets the provided function to be executed
// when a new subscription is added to the topic's subscribers list.
// The context passed into the provided function includes data of the
// new connection.
func (t *topic) OnSub(fn func(context.Context)) {
	t.events.sub.mu.Lock()
	t.events.sub.fns = append(t.events.sub.fns, fn)
	t.events.sub.mu.Unlock()
}

// execSubFns executes all sub event functions.
func (t *topic) execSubFns(ctx context.Context) {
	t.events.sub.mu.Lock()
	defer t.events.sub.mu.Unlock()

	for _, fn := range t.events.sub.fns {
		t.supv.Go(func(gctx context.Context) {
			mctx, cancel := ctxutil.MultiContext(ctx, gctx)
			defer cancel()

			fn(mctx)
		})
	}
}

// OnUnsub sets the provided function to be executed
// when a subscription is removed from the topic's subscribers list.
// The context passed into the provided function includes data of the
// removed connection.
func (t *topic) OnUnsub(fn func(context.Context)) {
	t.events.unsub.mu.Lock()
	t.events.unsub.fns = append(t.events.unsub.fns, fn)
	t.events.unsub.mu.Unlock()
}

// execUnsubFns executes all unsub event functions.
func (t *topic) execUnsubFns(ctx context.Context) {
	t.events.unsub.mu.Lock()
	defer t.events.unsub.mu.Unlock()

	for _, fn := range t.events.unsub.fns {
		t.supv.Go(func(gctx context.Context) {
			mctx, cancel := ctxutil.MultiContext(ctx, gctx)
			defer cancel()

			fn(mctx)
		})
	}
}

// OnFirstSub sets the provided function to be executed
// when the first subscription is added to the topic's
// subscribers list.
func (t *topic) OnFirstSub(fn func(context.Context)) {
	t.events.firstSub.mu.Lock()
	t.events.firstSub.fns = append(t.events.firstSub.fns, fn)
	t.events.firstSub.mu.Unlock()
}

// execFirstSubFns executes all first sub event functions.
func (t *topic) execFirstSubFns() {
	t.events.firstSub.mu.Lock()
	defer t.events.firstSub.mu.Unlock()

	for _, fn := range t.events.firstSub.fns {
		t.supv.Go(func(gctx context.Context) {
			fn(gctx)
		})
	}
}

// OnLastSub sets the provided function to be executed
// when the last subscription is removed to the topic's
// subscribers list.
func (t *topic) OnLastUnsub(fn func(context.Context)) {
	t.events.lastUnsub.mu.Lock()
	t.events.lastUnsub.fns = append(t.events.lastUnsub.fns, fn)
	t.events.lastUnsub.mu.Unlock()
}

// execLastUnsubFns executes all last unsub event functions.
func (t *topic) execLastUnsubFns() {
	t.events.lastUnsub.mu.Lock()
	defer t.events.lastUnsub.mu.Unlock()

	for _, fn := range t.events.lastUnsub.fns {
		t.supv.Go(func(gctx context.Context) {
			fn(gctx)
		})
	}
}

// pattern wraps a parsed topic and adds data fields for easier
// topic parameter detection.
type pattern struct {
	topic           wsutil.Topic
	path            []pathElem
	hasPlaceholders bool
}

// pathElem represents a single part of the topic's path.
type pathElem struct {
	placeholder bool
	value       string
}

// newPattern creates a topic pattern.
func newPattern(tpc wsutil.Topic) pattern {
	p := pattern{
		topic: tpc,
	}

	for _, elem := range tpc.Path {
		pElem := pathElem{
			value: elem,
		}

		if strings.HasPrefix(elem, "{") && strings.HasSuffix(elem, "}") {
			pElem.placeholder = true
			pElem.value = strings.TrimSuffix(strings.TrimPrefix(elem, "{"), "}")
			p.hasPlaceholders = true
		}

		p.path = append(p.path, pElem)
	}

	return p
}

// format assembles the topic's parts into a single string.
func (p pattern) format(params map[string]string) string {
	tpc := p.topic
	tpc.Path = nil

	for _, elem := range p.path {
		v := elem.value
		if elem.placeholder {
			v = params[v]
		}

		tpc.Path = append(tpc.Path, v)
	}

	return tpc.String()
}

type contextKey int

const _topicParamsKey contextKey = 1

// NewTopicParamsContext creates a new context with the topic parameters
// embedded into it.
func NewTopicParamsContext(ctx context.Context, params map[string]string) context.Context {
	if len(params) == 0 {
		return ctx
	}

	return context.WithValue(ctx, _topicParamsKey, params)
}

// TopicParamFromContext extracts a single topic parameter from the
// context.
func TopicParamFromContext(ctx context.Context, key string) string {
	params, _ := ctx.Value(_topicParamsKey).(map[string]string) // 'ok' is discarded because params length is checked one line below
	if len(params) == 0 {
		return ""
	}

	return params[key]
}

// areTopicParamsEqual checks if the two provided topic parameter maps
// are equal.
func areTopicParamsEqual(params1, params2 map[string]string) bool {
	for k1, v1 := range params1 {
		if v2, ok := params2[k1]; !ok || v2 != v1 {
			return false
		}
	}

	return true
}
