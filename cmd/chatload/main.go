// Command chatload drives the chat API with many live streams and a steady and
// bursty message load, then checks the capacity targets of
// docs/briefs/realtime-concurrent-chat.md section 5. It creates its own
// students directly in the database and removes them when it finishes.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	targetEndToEndP95 = 300 * time.Millisecond
	targetEndToEndP99 = time.Second
	targetSendP95     = 200 * time.Millisecond
	targetReconnect   = 30 * time.Second
	minSenderGap      = 2500 * time.Millisecond
	catchUpPageSize   = 50
	maxInFlight       = 400
)

type config struct {
	api        string
	dbURL      string
	streams    int
	groupShare float64
	groupSize  int
	rate       int
	duration   time.Duration
	burst      int
	burstFor   time.Duration
	slow       int
	deployCmd  string
	keep       bool
}

type student struct {
	id    uuid.UUID
	token string
}

type conversation struct {
	id      uuid.UUID
	members []int
}

type sentMessage struct {
	conversation uuid.UUID
	sender       uuid.UUID
	start        time.Time
}

type harness struct {
	cfg       config
	pool      *pgxpool.Pool
	api       *http.Client
	streams   *http.Client
	prefix    string
	students  []student
	direct    []conversation
	groups    []conversation
	clients   []*client
	slow      []*slowReader
	lastSend  []atomic.Int64
	sent      sync.Map
	sendMu    sync.Mutex
	sendTimes []time.Duration
	e2eMu     sync.Mutex
	e2e       []time.Duration
	statusMu  sync.Mutex
	statuses  map[int]int
	acked     atomic.Int64
	skipped   atomic.Int64
	deployAt  atomic.Int64
}

func main() {
	var cfg config
	flag.StringVar(&cfg.api, "api", "http://localhost:8080", "API base URL")
	flag.StringVar(&cfg.dbURL, "db", os.Getenv("DATABASE_URL"), "database URL, to create students and verify the result")
	flag.IntVar(&cfg.streams, "streams", 1000, "students with an open stream")
	flag.Float64Var(&cfg.groupShare, "group-share", 0.2, "share of messages sent in groups")
	flag.IntVar(&cfg.groupSize, "group-size", 25, "members per group, at most 50")
	flag.IntVar(&cfg.rate, "rate", 20, "messages per second in the steady phase")
	flag.DurationVar(&cfg.duration, "duration", 60*time.Second, "length of the steady phase")
	flag.IntVar(&cfg.burst, "burst", 100, "messages per second in the burst phase")
	flag.DurationVar(&cfg.burstFor, "burst-for", 30*time.Second, "length of the burst phase")
	flag.IntVar(&cfg.slow, "slow", 5, "group members whose stream is open but never read")
	flag.StringVar(&cfg.deployCmd, "deploy-cmd", "", "shell command that restarts the API; measures how fast every stream is back and caught up")
	flag.BoolVar(&cfg.keep, "keep", false, "keep the generated students and conversations")
	flag.Parse()

	if cfg.dbURL == "" {
		log.Fatal("chatload: -db or DATABASE_URL is required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, cfg); err != nil {
		log.Fatalf("chatload: %v", err)
	}
}

func run(ctx context.Context, cfg config) error {
	pool, err := pgxpool.New(ctx, cfg.dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	h := &harness{
		cfg:      cfg,
		pool:     pool,
		prefix:   "chatload-" + uuid.NewString()[:8] + "-",
		statuses: map[int]int{},
		api:      &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{MaxIdleConns: 1000, MaxIdleConnsPerHost: 1000}},
		streams:  &http.Client{Transport: &http.Transport{MaxIdleConns: 10, DisableCompression: true}},
	}
	if !cfg.keep {
		defer h.cleanup()
	}

	if err := h.setup(ctx); err != nil {
		return fmt.Errorf("setup: %w", err)
	}

	streamCtx, stopStreams := context.WithCancel(ctx)
	defer stopStreams()
	h.openStreams(streamCtx)
	if !h.waitReady(ctx, 60*time.Second) {
		return fmt.Errorf("streams did not all open")
	}
	log.Printf("%d streams open, %d slow readers", len(h.clients), len(h.slow))

	log.Printf("steady phase: %d msg/s for %v", cfg.rate, cfg.duration)
	h.phase(ctx, cfg.rate, cfg.duration)
	log.Printf("burst phase: %d msg/s for %v", cfg.burst, cfg.burstFor)
	h.phase(ctx, cfg.burst, cfg.burstFor)
	time.Sleep(3 * time.Second)

	var reconnect time.Duration
	if cfg.deployCmd != "" {
		reconnect = h.deploy(ctx)
	}

	return h.report(ctx, reconnect)
}

func (h *harness) call(ctx context.Context, method, path, token string, body any, headers ...string) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, h.cfg.api+path, reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := h.api.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	return resp.StatusCode, raw, err
}

func (h *harness) setup(ctx context.Context) error {
	n := h.cfg.streams
	groupUsers := int(float64(n) * 0.2)
	if h.cfg.groupShare == 0 {
		groupUsers = 0
	}
	groupUsers -= groupUsers % h.cfg.groupSize
	directUsers := n - groupUsers
	directUsers -= directUsers % 2

	rows, err := h.pool.Query(ctx, `
		INSERT INTO users (university_id, email, first_name, last_name)
		SELECT 'umcs', $1 || g || '@example.test', 'Load', 'Test' FROM generate_series(1, $2) g
		RETURNING id`, h.prefix, directUsers+groupUsers)
	if err != nil {
		return err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	batch := &pgx.Batch{}
	for _, id := range ids {
		token := auth.GenerateSessionToken()
		hash := token.Hash()
		h.students = append(h.students, student{id: id, token: token.String()})
		batch.Queue("INSERT INTO sessions (user_id, token_hash, expires_at) VALUES ($1, $2, NOW() + interval '6 hours')", id, hash[:])
	}
	for i := 0; i+1 < directUsers; i += 2 {
		batch.Queue(`INSERT INTO connections (user_low, user_high, mode) VALUES (LEAST($1::uuid, $2::uuid), GREATEST($1::uuid, $2::uuid), 'friends')`, ids[i], ids[i+1])
	}
	for start := directUsers; start+h.cfg.groupSize <= len(ids); start += h.cfg.groupSize {
		for j := start + 1; j < start+h.cfg.groupSize; j++ {
			batch.Queue(`INSERT INTO connections (user_low, user_high, mode) VALUES (LEAST($1::uuid, $2::uuid), GREATEST($1::uuid, $2::uuid), 'friends')`, ids[start], ids[j])
		}
	}
	results := h.pool.SendBatch(ctx, batch)
	for range batch.Len() {
		if _, err := results.Exec(); err != nil {
			_ = results.Close()
			return err
		}
	}
	if err := results.Close(); err != nil {
		return err
	}
	h.lastSend = make([]atomic.Int64, len(ids))

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 32)
	var firstErr error
	fail := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if firstErr == nil {
			firstErr = err
		}
	}
	for i := 0; i+1 < directUsers; i += 2 {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			status, raw, err := h.call(ctx, http.MethodPost, "/chat/conversations/direct", h.students[i].token, map[string]any{"userId": h.students[i+1].id})
			id, perr := idOf(raw)
			if err != nil || perr != nil || status/100 != 2 {
				fail(fmt.Errorf("open direct: %d %v %v %s", status, err, perr, raw))
				return
			}
			mu.Lock()
			h.direct = append(h.direct, conversation{id: id, members: []int{i, i + 1}})
			mu.Unlock()
		}()
	}
	for start := directUsers; start+h.cfg.groupSize <= len(ids); start += h.cfg.groupSize {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			members := make([]int, 0, h.cfg.groupSize)
			memberIDs := make([]uuid.UUID, 0, h.cfg.groupSize)
			for j := start; j < start+h.cfg.groupSize; j++ {
				members = append(members, j)
				if j > start {
					memberIDs = append(memberIDs, ids[j])
				}
			}
			status, raw, err := h.call(ctx, http.MethodPost, "/chat/conversations/groups", h.students[start].token, map[string]any{"name": "load group", "memberIds": memberIDs})
			id, perr := idOf(raw)
			if err != nil || perr != nil || status != http.StatusCreated {
				fail(fmt.Errorf("create group: %d %v %v %s", status, err, perr, raw))
				return
			}
			mu.Lock()
			h.groups = append(h.groups, conversation{id: id, members: members})
			mu.Unlock()
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	log.Printf("setup: %d students, %d direct conversations, %d groups of %d", len(ids), len(h.direct), len(h.groups), h.cfg.groupSize)
	return nil
}

func idOf(raw []byte) (uuid.UUID, error) {
	var v struct {
		ID uuid.UUID `json:"id"`
	}
	err := json.Unmarshal(raw, &v)
	return v.ID, err
}

func (h *harness) phase(ctx context.Context, rate int, d time.Duration) {
	start := time.Now()
	var launched int
	sem := make(chan struct{}, maxInFlight)
	var wg sync.WaitGroup
	for time.Since(start) < d && ctx.Err() == nil {
		due := int(time.Since(start).Seconds() * float64(rate))
		for ; launched < due; launched++ {
			select {
			case sem <- struct{}{}:
				wg.Add(1)
				go func() {
					defer wg.Done()
					defer func() { <-sem }()
					h.send(ctx)
				}()
			default:
				h.skipped.Add(1)
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	wg.Wait()
}

func (h *harness) pick() (conversation, int, bool) {
	for range 20 {
		pool := h.direct
		if len(h.groups) > 0 && rand.Float64() < h.cfg.groupShare {
			pool = h.groups
		}
		if len(pool) == 0 {
			continue
		}
		c := pool[rand.IntN(len(pool))]
		sender := c.members[rand.IntN(len(c.members))]
		last := h.lastSend[sender].Load()
		now := time.Now().UnixNano()
		if now-last < int64(minSenderGap) || !h.lastSend[sender].CompareAndSwap(last, now) {
			continue
		}
		return c, sender, true
	}
	return conversation{}, 0, false
}

func (h *harness) send(ctx context.Context) {
	c, sender, ok := h.pick()
	if !ok {
		h.skipped.Add(1)
		return
	}
	key := uuid.New()
	start := time.Now()
	h.sent.Store(key, sentMessage{conversation: c.id, sender: h.students[sender].id, start: start})
	status, _, err := h.call(ctx, http.MethodPost, "/chat/conversations/"+c.id.String()+"/messages", h.students[sender].token,
		map[string]any{"body": "load " + key.String()}, "Idempotency-Key", key.String())
	elapsed := time.Since(start)
	if err != nil {
		status = -1
	}
	h.statusMu.Lock()
	h.statuses[status]++
	h.statusMu.Unlock()
	if status == http.StatusCreated {
		h.acked.Add(1)
		h.sendMu.Lock()
		h.sendTimes = append(h.sendTimes, elapsed)
		h.sendMu.Unlock()
	}
}

type client struct {
	h       *harness
	index   int
	mu      sync.Mutex
	cursors map[uuid.UUID]int64
	got     map[uuid.UUID]int
	dupes   int
	ready   atomic.Bool
	backAt  atomic.Int64
	convs   map[uuid.UUID]bool
}

func (h *harness) openStreams(ctx context.Context) {
	slowFrom := h.firstGroupUser()
	slowSet := map[int]bool{}
	if slowFrom >= 0 {
		for i := 0; i < h.cfg.slow && slowFrom+1+i < len(h.students); i++ {
			slowSet[slowFrom+1+i] = true
		}
	}
	memberOf := map[int]map[uuid.UUID]bool{}
	for _, c := range append(slices.Clone(h.direct), h.groups...) {
		for _, m := range c.members {
			if memberOf[m] == nil {
				memberOf[m] = map[uuid.UUID]bool{}
			}
			memberOf[m][c.id] = true
		}
	}
	for i := range h.students {
		if slowSet[i] {
			s := &slowReader{h: h, index: i}
			h.slow = append(h.slow, s)
			go s.run(ctx)
			continue
		}
		c := &client{h: h, index: i, cursors: map[uuid.UUID]int64{}, got: map[uuid.UUID]int{}, convs: memberOf[i]}
		h.clients = append(h.clients, c)
		go c.run(ctx)
	}
}

func (h *harness) firstGroupUser() int {
	if len(h.groups) == 0 {
		return -1
	}
	return h.groups[0].members[0]
}

func (h *harness) waitReady(ctx context.Context, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		ready := 0
		for _, c := range h.clients {
			if c.ready.Load() {
				ready++
			}
		}
		if ready == len(h.clients) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func (c *client) run(ctx context.Context) {
	for ctx.Err() == nil {
		delay := c.stream(ctx)
		c.ready.Store(false)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

type eventPayload struct {
	ConversationID uuid.UUID `json:"conversationId"`
	AfterMs        int64     `json:"afterMs"`
	Message        struct {
		ID       uuid.UUID  `json:"id"`
		ClientID *uuid.UUID `json:"clientId"`
		Kind     string     `json:"kind"`
		Sender   struct {
			ID *uuid.UUID `json:"id"`
		} `json:"sender"`
	} `json:"message"`
}

func (c *client) stream(ctx context.Context) time.Duration {
	retry := time.Second + time.Duration(rand.IntN(1000))*time.Millisecond
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.h.cfg.api+"/chat/stream", nil)
	if err != nil {
		return retry
	}
	req.Header.Set("Authorization", "Bearer "+c.h.students[c.index].token)
	resp, err := c.h.streams.Do(req)
	if err != nil {
		return retry
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return retry
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	var name, data string
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		case line == "" && name != "":
			if delay, done := c.handle(ctx, name, data); done {
				return delay
			}
			name, data = "", ""
		}
	}
	return retry
}

func (c *client) handle(ctx context.Context, name, data string) (time.Duration, bool) {
	var p eventPayload
	_ = json.Unmarshal([]byte(data), &p)
	switch name {
	case "ready":
		go func() {
			if err := c.catchUp(ctx); err == nil {
				c.ready.Store(true)
				if at := c.h.deployAt.Load(); at != 0 && c.backAt.Load() == 0 {
					c.backAt.Store(time.Now().UnixNano())
				}
			}
		}()
	case "message.created":
		now := time.Now()
		if p.Message.Kind != "text" {
			return 0, false
		}
		c.mu.Lock()
		c.got[p.Message.ID]++
		if c.got[p.Message.ID] > 1 {
			c.dupes++
		}
		c.mu.Unlock()
		if p.Message.ClientID != nil && p.Message.Sender.ID != nil && *p.Message.Sender.ID != c.h.students[c.index].id {
			if v, ok := c.h.sent.Load(*p.Message.ClientID); ok {
				c.h.e2eMu.Lock()
				sm, _ := v.(sentMessage)
				c.h.e2e = append(c.h.e2e, now.Sub(sm.start))
				c.h.e2eMu.Unlock()
			}
		}
	case "reconnect":
		return time.Duration(p.AfterMs) * time.Millisecond, true
	case "auth_expired":
		return time.Minute, true
	}
	return 0, false
}

type listPage struct {
	Items []struct {
		ID        uuid.UUID `json:"id"`
		ChangeSeq int64     `json:"changeSeq"`
	} `json:"items"`
	NextCursor *string `json:"nextCursor"`
}

type messagePage struct {
	Items []struct {
		ID   uuid.UUID `json:"id"`
		Kind string    `json:"kind"`
	} `json:"items"`
	NextCursor *string `json:"nextCursor"`
}

func (c *client) catchUp(ctx context.Context) error {
	token := c.h.students[c.index].token
	cursor := ""
	for {
		path := fmt.Sprintf("/chat/conversations?limit=%d", catchUpPageSize)
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		status, raw, err := c.h.call(ctx, http.MethodGet, path, token, nil)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("list conversations: %d %v", status, err)
		}
		var page listPage
		if err := json.Unmarshal(raw, &page); err != nil {
			return err
		}
		for _, item := range page.Items {
			c.mu.Lock()
			from := c.cursors[item.ID]
			c.mu.Unlock()
			if item.ChangeSeq <= from {
				continue
			}
			if err := c.catchUpConversation(ctx, item.ID, from); err != nil {
				return err
			}
			c.mu.Lock()
			c.cursors[item.ID] = item.ChangeSeq
			c.mu.Unlock()
		}
		if page.NextCursor == nil {
			return nil
		}
		cursor = *page.NextCursor
	}
}

func (c *client) catchUpConversation(ctx context.Context, id uuid.UUID, from int64) error {
	token := c.h.students[c.index].token
	cursor := ""
	for {
		path := fmt.Sprintf("/chat/conversations/%s/messages?changedAfter=%d&limit=%d", id, from, catchUpPageSize)
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		status, raw, err := c.h.call(ctx, http.MethodGet, path, token, nil)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("list messages: %d %v", status, err)
		}
		var page messagePage
		if err := json.Unmarshal(raw, &page); err != nil {
			return err
		}
		c.mu.Lock()
		for _, m := range page.Items {
			if m.Kind == "text" {
				if _, ok := c.got[m.ID]; !ok {
					c.got[m.ID] = 0
				}
			}
		}
		c.mu.Unlock()
		if page.NextCursor == nil {
			return nil
		}
		cursor = *page.NextCursor
	}
}

type slowReader struct {
	h     *harness
	index int
	resp  *http.Response
}

func (s *slowReader) run(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.h.cfg.api+"/chat/stream", nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+s.h.students[s.index].token)
	resp, err := s.h.streams.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return
	}
	s.resp = resp
	<-ctx.Done()
}

func (s *slowReader) drainedByServer() bool {
	if s.resp == nil {
		return true
	}
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, s.resp.Body)
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(3 * time.Second):
		return false
	}
}

func (h *harness) deploy(ctx context.Context) time.Duration {
	log.Printf("deploy: %s", h.cfg.deployCmd)
	h.deployAt.Store(time.Now().UnixNano())
	start := time.Now()
	out, err := exec.CommandContext(ctx, "sh", "-c", h.cfg.deployCmd).CombinedOutput()
	if err != nil {
		log.Printf("deploy command failed: %v\n%s", err, out)
	}

	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		back := 0
		for _, c := range h.clients {
			if c.backAt.Load() != 0 {
				back++
			}
		}
		if back == len(h.clients) {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	var last int64
	back := 0
	for _, c := range h.clients {
		if at := c.backAt.Load(); at != 0 {
			back++
			last = max(last, at)
		}
	}
	log.Printf("deploy: %d of %d streams are back", back, len(h.clients))
	if back < len(h.clients) {
		return time.Since(start)
	}
	return time.Unix(0, last).Sub(start)
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	return sorted[min(len(sorted)-1, int(float64(len(sorted))*p))]
}

func verdict(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}

func (h *harness) report(ctx context.Context, reconnect time.Duration) error {
	fmt.Println()
	h.statusMu.Lock()
	fmt.Printf("send statuses: %v, acknowledged %d, skipped by the generator %d\n", h.statuses, h.acked.Load(), h.skipped.Load())
	h.statusMu.Unlock()

	h.sendMu.Lock()
	slices.Sort(h.sendTimes)
	sendP50, sendP95, sendP99 := percentile(h.sendTimes, 0.5), percentile(h.sendTimes, 0.95), percentile(h.sendTimes, 0.99)
	h.sendMu.Unlock()
	h.e2eMu.Lock()
	slices.Sort(h.e2e)
	e50, e95, e99 := percentile(h.e2e, 0.5), percentile(h.e2e, 0.95), percentile(h.e2e, 0.99)
	deliveries := len(h.e2e)
	h.e2eMu.Unlock()

	fmt.Printf("send response  p50 %v  p95 %v  p99 %v   (target p95 < %v) %s\n", sendP50, sendP95, sendP99, targetSendP95, verdict(sendP95 < targetSendP95))
	fmt.Printf("send to receive p50 %v  p95 %v  p99 %v over %d deliveries   (target p95 < %v, p99 < %v) %s\n",
		e50, e95, e99, deliveries, targetEndToEndP95, targetEndToEndP99, verdict(e95 < targetEndToEndP95 && e99 < targetEndToEndP99))

	lost, liveMissing, dupLive, err := h.verifyDelivery(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("live deliveries missing before catch-up: %d, lost after catch-up: %d, duplicate live events: %d   %s\n", liveMissing, lost, dupLive, verdict(lost == 0))

	stored, distinct, gaps, err := h.verifyStorage(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("stored text messages %d (acknowledged %d), distinct client ids %d, conversations with seq gaps or repeated change_seq: %d   %s\n",
		stored, h.acked.Load(), distinct, gaps, verdict(stored == int(h.acked.Load()) && stored == distinct && gaps == 0))

	closed := 0
	for _, s := range h.slow {
		if s.drainedByServer() {
			closed++
		}
	}
	fmt.Printf("slow readers closed by the server: %d of %d\n", closed, len(h.slow))
	if h.cfg.deployCmd != "" {
		fmt.Printf("all streams back and caught up after %v   (target < %v) %s\n", reconnect.Round(time.Millisecond), targetReconnect, verdict(reconnect < targetReconnect))
	}
	return nil
}

func (h *harness) expected(ctx context.Context) (map[uuid.UUID][]uuid.UUID, error) {
	var ids []uuid.UUID
	for _, c := range append(slices.Clone(h.direct), h.groups...) {
		ids = append(ids, c.id)
	}
	rows, err := h.pool.Query(ctx, "SELECT conversation_id, id FROM chat_messages WHERE conversation_id = ANY($1) AND kind = 'text' AND client_id IS NOT NULL", ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byConversation := map[uuid.UUID][]uuid.UUID{}
	for rows.Next() {
		var conversationID, id uuid.UUID
		if err := rows.Scan(&conversationID, &id); err != nil {
			return nil, err
		}
		byConversation[conversationID] = append(byConversation[conversationID], id)
	}
	return byConversation, rows.Err()
}

func (h *harness) verifyDelivery(ctx context.Context) (lost, liveMissing, dupLive int, err error) {
	expected, err := h.expected(ctx)
	if err != nil {
		return 0, 0, 0, err
	}
	missing := func(c *client) []uuid.UUID {
		c.mu.Lock()
		defer c.mu.Unlock()
		var out []uuid.UUID
		for conv := range c.convs {
			for _, id := range expected[conv] {
				if _, ok := c.got[id]; !ok {
					out = append(out, id)
				}
			}
		}
		return out
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, 64)
	for _, c := range h.clients {
		liveMissing += len(missing(c))
		dupLive += c.dupes
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := c.catchUp(ctx); err != nil {
				log.Printf("final catch-up: %v", err)
			}
			n := len(missing(c))
			mu.Lock()
			lost += n
			mu.Unlock()
		}()
	}
	wg.Wait()
	return lost, liveMissing, dupLive, nil
}

func (h *harness) verifyStorage(ctx context.Context) (stored, distinct, gaps int, err error) {
	var ids []uuid.UUID
	for _, c := range append(slices.Clone(h.direct), h.groups...) {
		ids = append(ids, c.id)
	}
	err = h.pool.QueryRow(ctx, "SELECT COUNT(*), COUNT(DISTINCT client_id) FROM chat_messages WHERE conversation_id = ANY($1) AND kind = 'text'", ids).Scan(&stored, &distinct)
	if err != nil {
		return 0, 0, 0, err
	}
	err = h.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM (
			SELECT conversation_id FROM chat_messages WHERE conversation_id = ANY($1)
			GROUP BY conversation_id
			HAVING MIN(seq) <> 1 OR MAX(seq) <> COUNT(*) OR COUNT(DISTINCT change_seq) <> COUNT(*)
		) bad`, ids).Scan(&gaps)
	return stored, distinct, gaps, err
}

func (h *harness) cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	statements := []string{
		"DELETE FROM chat_conversations WHERE id IN (SELECT m.conversation_id FROM chat_members m JOIN users u ON u.id = m.user_id WHERE u.email LIKE $1)",
		"DELETE FROM rate_limits WHERE key LIKE 'chat-%' AND split_part(key, ':', 2) IN (SELECT id::text FROM users WHERE email LIKE $1)",
		"DELETE FROM users WHERE email LIKE $1",
	}
	for _, s := range statements {
		if _, err := h.pool.Exec(ctx, s, h.prefix+"%"); err != nil {
			log.Printf("cleanup: %v", err)
		}
	}
	log.Printf("cleaned up %s*", h.prefix)
}
