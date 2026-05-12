package sessionstore

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/sandbox0-ai/sandbox0/storage-proxy/pkg/db"
)

type fakeRepo struct {
	checkpoints map[string]*db.SessionCheckpoint
	events      []*db.SessionEvent
	stage       []*db.SessionStageEntry
	refs        map[string]*db.SessionRef
	cursors     map[string]*db.SessionHarnessCursor
	createErr   error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		checkpoints: make(map[string]*db.SessionCheckpoint),
		refs:        make(map[string]*db.SessionRef),
		cursors:     make(map[string]*db.SessionHarnessCursor),
	}
}

func (r *fakeRepo) CreateSessionCheckpoint(_ context.Context, checkpoint *db.SessionCheckpoint) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.checkpoints[checkpoint.ID] = checkpoint
	return nil
}

func (r *fakeRepo) GetSessionCheckpoint(_ context.Context, id string) (*db.SessionCheckpoint, error) {
	checkpoint, ok := r.checkpoints[id]
	if !ok {
		return nil, db.ErrNotFound
	}
	return checkpoint, nil
}

func (r *fakeRepo) GetSmartestSessionCheckpoint(_ context.Context, teamID, sessionID string) (*db.SessionCheckpoint, error) {
	var best *db.SessionCheckpoint
	for _, checkpoint := range r.checkpoints {
		if checkpoint.TeamID != teamID || checkpoint.SessionID != sessionID || checkpoint.Kind != db.SessionCheckpointKindSmartest {
			continue
		}
		if best == nil || checkpoint.Score > best.Score || checkpoint.Score == best.Score && checkpoint.CreatedAt.After(best.CreatedAt) {
			best = checkpoint
		}
	}
	if best == nil {
		return nil, db.ErrNotFound
	}
	return best, nil
}

func (r *fakeRepo) ListSessionCheckpoints(_ context.Context, teamID, sessionID string) ([]*db.SessionCheckpoint, error) {
	var checkpoints []*db.SessionCheckpoint
	for _, checkpoint := range r.checkpoints {
		if checkpoint.TeamID == teamID && checkpoint.SessionID == sessionID {
			checkpoints = append(checkpoints, checkpoint)
		}
	}
	return checkpoints, nil
}

func (r *fakeRepo) AppendSessionEvent(_ context.Context, event *db.SessionEvent) (*db.SessionEvent, error) {
	var maxSeq int64
	for _, existing := range r.events {
		if existing.TeamID == event.TeamID && existing.SessionID == event.SessionID && existing.Seq > maxSeq {
			maxSeq = existing.Seq
		}
	}
	copy := *event
	copy.Seq = maxSeq + 1
	r.events = append(r.events, &copy)
	return &copy, nil
}

func (r *fakeRepo) ListSessionEvents(_ context.Context, teamID, sessionID string, afterSeq, beforeSeq int64, limit int) ([]*db.SessionEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	var events []*db.SessionEvent
	for _, event := range r.events {
		if event.TeamID != teamID || event.SessionID != sessionID {
			continue
		}
		if afterSeq > 0 && event.Seq <= afterSeq {
			continue
		}
		if beforeSeq > 0 && event.Seq > beforeSeq {
			continue
		}
		events = append(events, event)
		if len(events) == limit {
			break
		}
	}
	return events, nil
}

func (r *fakeRepo) GetLastSessionEventSeq(_ context.Context, teamID, sessionID string) (int64, error) {
	var seq int64
	for _, event := range r.events {
		if event.TeamID == teamID && event.SessionID == sessionID && event.Seq > seq {
			seq = event.Seq
		}
	}
	return seq, nil
}

func (r *fakeRepo) CreateSessionStageEntry(_ context.Context, entry *db.SessionStageEntry) error {
	r.stage = append(r.stage, entry)
	return nil
}

func (r *fakeRepo) ListSessionStageEntries(_ context.Context, teamID, sessionID string) ([]*db.SessionStageEntry, error) {
	var entries []*db.SessionStageEntry
	for _, entry := range r.stage {
		if entry.TeamID == teamID && entry.SessionID == sessionID {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func (r *fakeRepo) ClearSessionStageEntries(_ context.Context, teamID, sessionID string) error {
	var kept []*db.SessionStageEntry
	for _, entry := range r.stage {
		if entry.TeamID == teamID && entry.SessionID == sessionID {
			continue
		}
		kept = append(kept, entry)
	}
	r.stage = kept
	return nil
}

func refKey(teamID, sessionID, refType, name string) string {
	return teamID + "/" + sessionID + "/" + refType + "/" + name
}

func (r *fakeRepo) UpsertSessionRef(_ context.Context, ref *db.SessionRef) error {
	copy := *ref
	r.refs[refKey(ref.TeamID, ref.SessionID, ref.RefType, ref.Name)] = &copy
	return nil
}

func (r *fakeRepo) GetSessionRef(_ context.Context, teamID, sessionID, refType, name string) (*db.SessionRef, error) {
	ref, ok := r.refs[refKey(teamID, sessionID, refType, name)]
	if !ok {
		return nil, db.ErrNotFound
	}
	return ref, nil
}

func cursorKey(teamID, sessionID, harnessID string) string {
	return teamID + "/" + sessionID + "/" + harnessID
}

func (r *fakeRepo) UpsertSessionHarnessCursor(_ context.Context, cursor *db.SessionHarnessCursor) error {
	copy := *cursor
	r.cursors[cursorKey(cursor.TeamID, cursor.SessionID, cursor.HarnessID)] = &copy
	return nil
}

func (r *fakeRepo) GetSessionHarnessCursor(_ context.Context, teamID, sessionID, harnessID string) (*db.SessionHarnessCursor, error) {
	cursor, ok := r.cursors[cursorKey(teamID, sessionID, harnessID)]
	if !ok {
		return nil, db.ErrNotFound
	}
	return cursor, nil
}

type fakeSnapshotBackend struct {
	createReq   *CreateSnapshotBackendRequest
	deleteCalls int
	forkReq     *ForkSnapshotBackendRequest
	restoreReq  *RestoreSnapshotBackendRequest
}

func (b *fakeSnapshotBackend) CreateSnapshot(_ context.Context, req *CreateSnapshotBackendRequest) (*db.Snapshot, error) {
	b.createReq = req
	return &db.Snapshot{
		ID:        "snap-1",
		VolumeID:  req.VolumeID,
		TeamID:    req.TeamID,
		UserID:    req.UserID,
		Name:      req.Name,
		CreatedAt: time.Now().UTC(),
	}, nil
}

func (b *fakeSnapshotBackend) DeleteSnapshot(context.Context, string, string, string) error {
	b.deleteCalls++
	return nil
}

func (b *fakeSnapshotBackend) ForkSnapshot(_ context.Context, req *ForkSnapshotBackendRequest) (*db.SandboxVolume, error) {
	b.forkReq = req
	sourceID := req.VolumeID
	return &db.SandboxVolume{
		ID:             "vol-clone",
		TeamID:         req.TeamID,
		UserID:         req.UserID,
		SourceVolumeID: &sourceID,
	}, nil
}

func (b *fakeSnapshotBackend) RestoreSnapshot(_ context.Context, req *RestoreSnapshotBackendRequest) error {
	b.restoreReq = req
	return nil
}

func TestCreateCheckpointCreatesSnapshotAndMetadata(t *testing.T) {
	repo := newFakeRepo()
	backend := &fakeSnapshotBackend{}
	service := NewService(repo, backend)
	service.now = func() time.Time { return time.Date(2026, 5, 12, 1, 2, 3, 0, time.UTC) }

	checkpoint, err := service.CreateCheckpoint(context.Background(), CreateCheckpointRequest{
		SessionID: "sess-1",
		VolumeID:  "vol-1",
		TeamID:    "team-1",
		UserID:    "user-1",
		Label:     "repo indexed",
		Kind:      db.SessionCheckpointKindSmartest,
		Score:     0.95,
	})
	if err != nil {
		t.Fatalf("CreateCheckpoint() error = %v", err)
	}
	if backend.createReq == nil {
		t.Fatal("expected snapshot to be created")
	}
	if checkpoint.SnapshotID != "snap-1" || checkpoint.Kind != db.SessionCheckpointKindSmartest || checkpoint.Score != 0.95 {
		t.Fatalf("unexpected checkpoint: %+v", checkpoint)
	}
	if _, ok := repo.checkpoints[checkpoint.ID]; !ok {
		t.Fatal("checkpoint was not persisted")
	}
}

func TestCreateCheckpointDeletesSnapshotWhenMetadataWriteFails(t *testing.T) {
	repo := newFakeRepo()
	repo.createErr = errors.New("db down")
	backend := &fakeSnapshotBackend{}
	service := NewService(repo, backend)

	_, err := service.CreateCheckpoint(context.Background(), CreateCheckpointRequest{
		SessionID: "sess-1",
		VolumeID:  "vol-1",
		TeamID:    "team-1",
		UserID:    "user-1",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if backend.deleteCalls != 1 {
		t.Fatalf("deleteCalls = %d, want 1", backend.deleteCalls)
	}
}

func TestGetSmartestCheckpointSelectsHighestScore(t *testing.T) {
	repo := newFakeRepo()
	repo.checkpoints["low"] = &db.SessionCheckpoint{ID: "low", TeamID: "team-1", SessionID: "sess-1", Kind: db.SessionCheckpointKindSmartest, Score: 0.1}
	repo.checkpoints["high"] = &db.SessionCheckpoint{ID: "high", TeamID: "team-1", SessionID: "sess-1", Kind: db.SessionCheckpointKindSmartest, Score: 0.9}
	service := NewService(repo, &fakeSnapshotBackend{})

	checkpoint, err := service.GetSmartestCheckpoint(context.Background(), "team-1", "sess-1")
	if err != nil {
		t.Fatalf("GetSmartestCheckpoint() error = %v", err)
	}
	if checkpoint.ID != "high" {
		t.Fatalf("checkpoint = %q, want high", checkpoint.ID)
	}
}

func TestCloneFromCheckpointForksSnapshot(t *testing.T) {
	repo := newFakeRepo()
	repo.checkpoints["chk-1"] = &db.SessionCheckpoint{
		ID:         "chk-1",
		SessionID:  "sess-1",
		TeamID:     "team-1",
		UserID:     "user-1",
		VolumeID:   "vol-1",
		SnapshotID: "snap-1",
	}
	backend := &fakeSnapshotBackend{}
	service := NewService(repo, backend)

	result, err := service.CloneFromCheckpoint(context.Background(), CloneCheckpointRequest{
		SessionID:    "sess-1",
		CheckpointID: "chk-1",
		TeamID:       "team-1",
		UserID:       "worker-user",
	})
	if err != nil {
		t.Fatalf("CloneFromCheckpoint() error = %v", err)
	}
	if backend.forkReq == nil || backend.forkReq.SnapshotID != "snap-1" || backend.forkReq.UserID != "worker-user" {
		t.Fatalf("unexpected fork request: %+v", backend.forkReq)
	}
	if result.Volume.ID != "vol-clone" || result.SourceVolumeID != "vol-1" {
		t.Fatalf("unexpected clone result: %+v", result)
	}
}

func TestRollbackToCheckpointRestoresSnapshot(t *testing.T) {
	repo := newFakeRepo()
	repo.checkpoints["chk-1"] = &db.SessionCheckpoint{
		ID:         "chk-1",
		SessionID:  "sess-1",
		TeamID:     "team-1",
		UserID:     "user-1",
		VolumeID:   "vol-1",
		SnapshotID: "snap-1",
	}
	backend := &fakeSnapshotBackend{}
	service := NewService(repo, backend)

	_, err := service.RollbackToCheckpoint(context.Background(), RollbackCheckpointRequest{
		SessionID:    "sess-1",
		CheckpointID: "chk-1",
		TeamID:       "team-1",
	})
	if err != nil {
		t.Fatalf("RollbackToCheckpoint() error = %v", err)
	}
	if backend.restoreReq == nil || backend.restoreReq.VolumeID != "vol-1" || backend.restoreReq.SnapshotID != "snap-1" {
		t.Fatalf("unexpected restore request: %+v", backend.restoreReq)
	}
}

func TestCheckpointOwnershipMismatch(t *testing.T) {
	repo := newFakeRepo()
	repo.checkpoints["chk-1"] = &db.SessionCheckpoint{ID: "chk-1", SessionID: "sess-1", TeamID: "team-1"}
	service := NewService(repo, &fakeSnapshotBackend{})

	_, err := service.CloneFromCheckpoint(context.Background(), CloneCheckpointRequest{
		SessionID:    "other-session",
		CheckpointID: "chk-1",
		TeamID:       "team-1",
	})
	if !errors.Is(err, ErrCheckpointOwnership) {
		t.Fatalf("error = %v, want %v", err, ErrCheckpointOwnership)
	}
}

func TestEventsRemainSliceableLikeObjectLog(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, &fakeSnapshotBackend{})

	for i := 1; i <= 80; i++ {
		payload := json.RawMessage(`{"step":` + string(rune('0'+(i%10))) + `}`)
		event, err := service.AppendEvent(context.Background(), AppendEventRequest{
			SessionID: "sess-1",
			TeamID:    "team-1",
			EventType: "tool.result",
			Payload:   payload,
		})
		if err != nil {
			t.Fatalf("AppendEvent(%d) error = %v", i, err)
		}
		if event.Seq != int64(i) {
			t.Fatalf("event seq = %d, want %d", event.Seq, i)
		}
	}

	events, err := service.GetEvents(context.Background(), "team-1", "sess-1", EventRange{AfterSeq: 59, Limit: 100})
	if err != nil {
		t.Fatalf("GetEvents() error = %v", err)
	}
	if len(events) != 21 || events[0].Seq != 60 || events[len(events)-1].Seq != 80 {
		t.Fatalf("unexpected slice: len=%d first=%d last=%d", len(events), events[0].Seq, events[len(events)-1].Seq)
	}

	early, err := service.GetEvents(context.Background(), "team-1", "sess-1", EventRange{AfterSeq: 14, BeforeSeq: 15, Limit: 1})
	if err != nil {
		t.Fatalf("GetEvents(early) error = %v", err)
	}
	if len(early) != 1 || early[0].Seq != 15 {
		t.Fatalf("early event = %+v, want seq 15", early)
	}
}

func TestAddCommitTagCheckoutAndForkRef(t *testing.T) {
	repo := newFakeRepo()
	backend := &fakeSnapshotBackend{}
	service := NewService(repo, backend)

	for i := 0; i < 3; i++ {
		if _, err := service.AppendEvent(context.Background(), AppendEventRequest{
			SessionID: "sess-1",
			TeamID:    "team-1",
			EventType: "agent.message",
			Payload:   json.RawMessage(`{"ok":true}`),
		}); err != nil {
			t.Fatalf("AppendEvent() error = %v", err)
		}
	}
	if _, err := service.Add(context.Background(), AddRequest{
		SessionID: "sess-1",
		TeamID:    "team-1",
		Selector:  StageSelector{Type: "range", AfterSeq: 1, BeforeSeq: 3},
		Note:      "clean context",
	}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	checkpoint, err := service.Commit(context.Background(), CommitRequest{
		SessionID: "sess-1",
		TeamID:    "team-1",
		UserID:    "user-1",
		VolumeID:  "vol-1",
		Message:   "repo indexed",
		Kind:      db.SessionCheckpointKindSmartest,
		Score:     0.99,
		RefType:   db.SessionRefTypeBranch,
		RefName:   "main",
	})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if checkpoint.EventSeq != 3 || checkpoint.ContextRecipe == nil {
		t.Fatalf("unexpected checkpoint: %+v", checkpoint)
	}
	if len(repo.stage) != 0 {
		t.Fatalf("stage was not cleared")
	}

	if _, err := service.Tag(context.Background(), "team-1", "sess-1", "smartest", checkpoint.ID); err != nil {
		t.Fatalf("Tag() error = %v", err)
	}
	checkedOut, err := service.Checkout(context.Background(), CheckoutRequest{
		SessionID: "sess-1",
		TeamID:    "team-1",
		Ref:       "smartest",
	})
	if err != nil {
		t.Fatalf("Checkout() error = %v", err)
	}
	if checkedOut.Checkpoint.ID != checkpoint.ID || len(checkedOut.Events) != 3 || checkedOut.Recipe == nil {
		t.Fatalf("unexpected checkout: %+v", checkedOut)
	}

	result, err := service.ForkRef(context.Background(), ForkRefRequest{
		SessionID: "sess-1",
		TeamID:    "team-1",
		UserID:    "worker-1",
		Ref:       "smartest",
	})
	if err != nil {
		t.Fatalf("ForkRef() error = %v", err)
	}
	if result.CheckpointID != checkpoint.ID || backend.forkReq == nil || backend.forkReq.SnapshotID != checkpoint.SnapshotID {
		t.Fatalf("unexpected fork result: %+v fork=%+v", result, backend.forkReq)
	}
}

func TestWakeResumesAfterHarnessCursor(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, &fakeSnapshotBackend{})

	for i := 0; i < 5; i++ {
		if _, err := service.AppendEvent(context.Background(), AppendEventRequest{
			SessionID: "sess-1",
			TeamID:    "team-1",
			EventType: "agent.message",
			Payload:   json.RawMessage(`{"step":true}`),
		}); err != nil {
			t.Fatalf("AppendEvent() error = %v", err)
		}
	}
	if _, err := service.UpdateCursor(context.Background(), UpdateCursorRequest{
		SessionID:   "sess-1",
		TeamID:      "team-1",
		HarnessID:   "harness-1",
		LastSeenSeq: 3,
	}); err != nil {
		t.Fatalf("UpdateCursor() error = %v", err)
	}

	wake, err := service.Wake(context.Background(), WakeRequest{
		SessionID: "sess-1",
		TeamID:    "team-1",
		HarnessID: "harness-1",
	})
	if err != nil {
		t.Fatalf("Wake() error = %v", err)
	}
	if wake.Cursor == nil || wake.Cursor.LastSeenSeq != 3 {
		t.Fatalf("unexpected cursor: %+v", wake.Cursor)
	}
	if len(wake.Events) != 2 || wake.Events[0].Seq != 4 || wake.Events[1].Seq != 5 {
		t.Fatalf("unexpected wake events: %+v", wake.Events)
	}
}
