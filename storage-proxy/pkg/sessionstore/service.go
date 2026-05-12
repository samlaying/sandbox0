package sessionstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sandbox0-ai/sandbox0/storage-proxy/pkg/db"
)

var (
	ErrInvalidRequest      = errors.New("invalid session checkpoint request")
	ErrCheckpointNotFound  = errors.New("session checkpoint not found")
	ErrCheckpointOwnership = errors.New("session checkpoint does not belong to session")
	ErrRefNotFound         = errors.New("session ref not found")
)

type Repository interface {
	CreateSessionCheckpoint(context.Context, *db.SessionCheckpoint) error
	GetSessionCheckpoint(context.Context, string) (*db.SessionCheckpoint, error)
	GetSmartestSessionCheckpoint(context.Context, string, string) (*db.SessionCheckpoint, error)
	ListSessionCheckpoints(context.Context, string, string) ([]*db.SessionCheckpoint, error)
	AppendSessionEvent(context.Context, *db.SessionEvent) (*db.SessionEvent, error)
	ListSessionEvents(context.Context, string, string, int64, int64, int) ([]*db.SessionEvent, error)
	GetLastSessionEventSeq(context.Context, string, string) (int64, error)
	CreateSessionStageEntry(context.Context, *db.SessionStageEntry) error
	ListSessionStageEntries(context.Context, string, string) ([]*db.SessionStageEntry, error)
	ClearSessionStageEntries(context.Context, string, string) error
	UpsertSessionRef(context.Context, *db.SessionRef) error
	GetSessionRef(context.Context, string, string, string, string) (*db.SessionRef, error)
	UpsertSessionHarnessCursor(context.Context, *db.SessionHarnessCursor) error
	GetSessionHarnessCursor(context.Context, string, string, string) (*db.SessionHarnessCursor, error)
}

type SnapshotBackend interface {
	CreateSnapshot(context.Context, *CreateSnapshotBackendRequest) (*db.Snapshot, error)
	DeleteSnapshot(context.Context, string, string, string) error
	ForkSnapshot(context.Context, *ForkSnapshotBackendRequest) (*db.SandboxVolume, error)
	RestoreSnapshot(context.Context, *RestoreSnapshotBackendRequest) error
}

type Service struct {
	repo     Repository
	snapshot SnapshotBackend
	now      func() time.Time
}

func NewService(repo Repository, snapshotBackend SnapshotBackend) *Service {
	return &Service{
		repo:     repo,
		snapshot: snapshotBackend,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

type CreateCheckpointRequest struct {
	SessionID          string
	VolumeID           string
	TeamID             string
	UserID             string
	ParentCheckpointID *string
	EventSeq           int64
	Label              string
	Kind               string
	Score              float64
	CreatedFromEventID *string
	ContextRecipe      *json.RawMessage
	Metadata           *json.RawMessage
}

type AppendEventRequest struct {
	SessionID string
	TeamID    string
	EventType string
	Payload   json.RawMessage
	Metadata  *json.RawMessage
}

type EventRange struct {
	AfterSeq  int64 `json:"after_seq,omitempty"`
	BeforeSeq int64 `json:"before_seq,omitempty"`
	Limit     int   `json:"limit,omitempty"`
}

type StageSelector struct {
	Type      string          `json:"type"`
	AfterSeq  int64           `json:"after_seq,omitempty"`
	BeforeSeq int64           `json:"before_seq,omitempty"`
	EventID   string          `json:"event_id,omitempty"`
	EventType string          `json:"event_type,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

type AddRequest struct {
	SessionID string
	TeamID    string
	Selector  StageSelector
	Note      string
}

type CommitRequest struct {
	SessionID          string
	TeamID             string
	UserID             string
	VolumeID           string
	Message            string
	Kind               string
	Score              float64
	ParentCheckpointID *string
	RefType            string
	RefName            string
}

type CheckoutRequest struct {
	SessionID string
	TeamID    string
	Ref       string
}

type CheckoutResult struct {
	Checkpoint *db.SessionCheckpoint `json:"checkpoint"`
	Events     []*db.SessionEvent    `json:"events"`
	Recipe     *json.RawMessage      `json:"recipe,omitempty"`
}

type ForkRefRequest struct {
	SessionID       string
	TeamID          string
	UserID          string
	Ref             string
	AccessMode      *string
	DefaultPosixUID *int64
	DefaultPosixGID *int64
}

type WakeRequest struct {
	SessionID string
	TeamID    string
	HarnessID string
	Limit     int
}

type WakeResult struct {
	Cursor *db.SessionHarnessCursor `json:"cursor,omitempty"`
	Events []*db.SessionEvent       `json:"events"`
}

type UpdateCursorRequest struct {
	SessionID   string
	TeamID      string
	HarnessID   string
	LastSeenSeq int64
	State       *json.RawMessage
}

type CreateSnapshotBackendRequest struct {
	VolumeID    string
	Name        string
	Description string
	TeamID      string
	UserID      string
}

type ForkSnapshotBackendRequest struct {
	VolumeID        string
	SnapshotID      string
	TeamID          string
	UserID          string
	AccessMode      *string
	DefaultPosixUID *int64
	DefaultPosixGID *int64
}

type RestoreSnapshotBackendRequest struct {
	VolumeID   string
	SnapshotID string
	TeamID     string
	UserID     string
}

type CloneCheckpointRequest struct {
	SessionID       string
	CheckpointID    string
	TeamID          string
	UserID          string
	AccessMode      *string
	DefaultPosixUID *int64
	DefaultPosixGID *int64
}

type CloneCheckpointResult struct {
	SourceSessionID string            `json:"source_session_id"`
	CheckpointID    string            `json:"checkpoint_id"`
	SnapshotID      string            `json:"snapshot_id"`
	SourceVolumeID  string            `json:"source_volume_id"`
	Volume          *db.SandboxVolume `json:"volume"`
}

type RollbackCheckpointRequest struct {
	SessionID    string
	CheckpointID string
	TeamID       string
	UserID       string
}

func (s *Service) CreateCheckpoint(ctx context.Context, req CreateCheckpointRequest) (*db.SessionCheckpoint, error) {
	if s == nil || s.repo == nil || s.snapshot == nil {
		return nil, fmt.Errorf("%w: service is not configured", ErrInvalidRequest)
	}
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.VolumeID = strings.TrimSpace(req.VolumeID)
	req.TeamID = strings.TrimSpace(req.TeamID)
	req.UserID = strings.TrimSpace(req.UserID)
	req.Label = strings.TrimSpace(req.Label)
	req.Kind = normalizeKind(req.Kind)
	if req.SessionID == "" || req.VolumeID == "" || req.TeamID == "" || req.UserID == "" {
		return nil, fmt.Errorf("%w: session_id, volume_id, team_id and user_id are required", ErrInvalidRequest)
	}
	if req.Label == "" {
		req.Label = "checkpoint"
	}
	if req.EventSeq == 0 {
		eventSeq, err := s.repo.GetLastSessionEventSeq(ctx, req.TeamID, req.SessionID)
		if err != nil {
			return nil, err
		}
		req.EventSeq = eventSeq
	}

	snap, err := s.snapshot.CreateSnapshot(ctx, &CreateSnapshotBackendRequest{
		VolumeID:    req.VolumeID,
		Name:        snapshotName(req.SessionID, req.Label),
		Description: fmt.Sprintf("session checkpoint %q for session %s", req.Label, req.SessionID),
		TeamID:      req.TeamID,
		UserID:      req.UserID,
	})
	if err != nil {
		return nil, err
	}

	checkpoint := &db.SessionCheckpoint{
		ID:                 uuid.New().String(),
		SessionID:          req.SessionID,
		TeamID:             req.TeamID,
		UserID:             req.UserID,
		VolumeID:           req.VolumeID,
		SnapshotID:         snap.ID,
		ParentCheckpointID: req.ParentCheckpointID,
		EventSeq:           req.EventSeq,
		Label:              req.Label,
		Kind:               req.Kind,
		Score:              req.Score,
		CreatedFromEventID: req.CreatedFromEventID,
		ContextRecipe:      req.ContextRecipe,
		Metadata:           req.Metadata,
		CreatedAt:          s.now(),
	}
	if err := s.repo.CreateSessionCheckpoint(ctx, checkpoint); err != nil {
		_ = s.snapshot.DeleteSnapshot(context.Background(), req.VolumeID, snap.ID, req.TeamID)
		return nil, err
	}
	return checkpoint, nil
}

func (s *Service) AppendEvent(ctx context.Context, req AppendEventRequest) (*db.SessionEvent, error) {
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.TeamID = strings.TrimSpace(req.TeamID)
	req.EventType = strings.TrimSpace(req.EventType)
	if req.SessionID == "" || req.TeamID == "" || req.EventType == "" || len(req.Payload) == 0 {
		return nil, fmt.Errorf("%w: session_id, team_id, event_type and payload are required", ErrInvalidRequest)
	}
	event := &db.SessionEvent{
		ID:        uuid.New().String(),
		SessionID: req.SessionID,
		TeamID:    req.TeamID,
		EventType: req.EventType,
		Payload:   cloneRaw(req.Payload),
		Metadata:  cloneRawPtr(req.Metadata),
		CreatedAt: s.now(),
	}
	return s.repo.AppendSessionEvent(ctx, event)
}

func (s *Service) GetEvents(ctx context.Context, teamID, sessionID string, r EventRange) ([]*db.SessionEvent, error) {
	return s.repo.ListSessionEvents(ctx, strings.TrimSpace(teamID), strings.TrimSpace(sessionID), r.AfterSeq, r.BeforeSeq, r.Limit)
}

func (s *Service) Add(ctx context.Context, req AddRequest) (*db.SessionStageEntry, error) {
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.TeamID = strings.TrimSpace(req.TeamID)
	req.Selector.Type = strings.TrimSpace(req.Selector.Type)
	if req.SessionID == "" || req.TeamID == "" || req.Selector.Type == "" {
		return nil, fmt.Errorf("%w: session_id, team_id and selector.type are required", ErrInvalidRequest)
	}
	selector, err := json.Marshal(req.Selector)
	if err != nil {
		return nil, err
	}
	entry := &db.SessionStageEntry{
		ID:        uuid.New().String(),
		SessionID: req.SessionID,
		TeamID:    req.TeamID,
		Selector:  selector,
		Note:      strings.TrimSpace(req.Note),
		CreatedAt: s.now(),
	}
	return entry, s.repo.CreateSessionStageEntry(ctx, entry)
}

func (s *Service) Commit(ctx context.Context, req CommitRequest) (*db.SessionCheckpoint, error) {
	staged, err := s.repo.ListSessionStageEntries(ctx, strings.TrimSpace(req.TeamID), strings.TrimSpace(req.SessionID))
	if err != nil {
		return nil, err
	}
	recipe, err := buildContextRecipe(staged)
	if err != nil {
		return nil, err
	}
	checkpoint, err := s.CreateCheckpoint(ctx, CreateCheckpointRequest{
		SessionID:          req.SessionID,
		VolumeID:           req.VolumeID,
		TeamID:             req.TeamID,
		UserID:             req.UserID,
		ParentCheckpointID: req.ParentCheckpointID,
		Label:              req.Message,
		Kind:               req.Kind,
		Score:              req.Score,
		ContextRecipe:      recipe,
	})
	if err != nil {
		return nil, err
	}
	if err := s.repo.ClearSessionStageEntries(ctx, strings.TrimSpace(req.TeamID), strings.TrimSpace(req.SessionID)); err != nil {
		return nil, err
	}
	refType := normalizeRefType(req.RefType)
	refName := strings.TrimSpace(req.RefName)
	if refName != "" {
		if err := s.upsertRef(ctx, req.TeamID, req.SessionID, refType, refName, checkpoint.ID); err != nil {
			return nil, err
		}
	}
	return checkpoint, nil
}

func (s *Service) Tag(ctx context.Context, teamID, sessionID, name, target string) (*db.SessionRef, error) {
	checkpoint, err := s.ResolveRef(ctx, teamID, sessionID, target)
	if err != nil {
		return nil, err
	}
	ref := &db.SessionRef{
		SessionID:    strings.TrimSpace(sessionID),
		TeamID:       strings.TrimSpace(teamID),
		RefType:      db.SessionRefTypeTag,
		Name:         strings.TrimSpace(name),
		CheckpointID: checkpoint.ID,
		CreatedAt:    s.now(),
		UpdatedAt:    s.now(),
	}
	return ref, s.repo.UpsertSessionRef(ctx, ref)
}

func (s *Service) Branch(ctx context.Context, teamID, sessionID, name, target string) (*db.SessionRef, error) {
	checkpoint, err := s.ResolveRef(ctx, teamID, sessionID, target)
	if err != nil {
		return nil, err
	}
	ref := &db.SessionRef{
		SessionID:    strings.TrimSpace(sessionID),
		TeamID:       strings.TrimSpace(teamID),
		RefType:      db.SessionRefTypeBranch,
		Name:         strings.TrimSpace(name),
		CheckpointID: checkpoint.ID,
		CreatedAt:    s.now(),
		UpdatedAt:    s.now(),
	}
	return ref, s.repo.UpsertSessionRef(ctx, ref)
}

func (s *Service) ResolveRef(ctx context.Context, teamID, sessionID, ref string) (*db.SessionCheckpoint, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || ref == "HEAD" || ref == db.SessionCheckpointKindSmartest {
		return s.GetSmartestCheckpoint(ctx, teamID, sessionID)
	}
	if checkpoint, err := s.repo.GetSessionCheckpoint(ctx, ref); err == nil {
		if checkpoint.TeamID != strings.TrimSpace(teamID) || checkpoint.SessionID != strings.TrimSpace(sessionID) {
			return nil, ErrCheckpointOwnership
		}
		return checkpoint, nil
	} else if !errors.Is(err, db.ErrNotFound) {
		return nil, err
	}
	for _, refType := range []string{db.SessionRefTypeTag, db.SessionRefTypeBranch} {
		named, err := s.repo.GetSessionRef(ctx, strings.TrimSpace(teamID), strings.TrimSpace(sessionID), refType, ref)
		if err == nil {
			return s.loadOwnedCheckpoint(ctx, teamID, sessionID, named.CheckpointID)
		}
		if !errors.Is(err, db.ErrNotFound) {
			return nil, err
		}
	}
	return nil, ErrRefNotFound
}

func (s *Service) Checkout(ctx context.Context, req CheckoutRequest) (*CheckoutResult, error) {
	checkpoint, err := s.ResolveRef(ctx, req.TeamID, req.SessionID, req.Ref)
	if err != nil {
		return nil, err
	}
	events, err := s.GetEvents(ctx, checkpoint.TeamID, checkpoint.SessionID, EventRange{BeforeSeq: checkpoint.EventSeq, Limit: 1000})
	if err != nil {
		return nil, err
	}
	return &CheckoutResult{Checkpoint: checkpoint, Events: events, Recipe: checkpoint.ContextRecipe}, nil
}

func (s *Service) ForkRef(ctx context.Context, req ForkRefRequest) (*CloneCheckpointResult, error) {
	checkpoint, err := s.ResolveRef(ctx, req.TeamID, req.SessionID, req.Ref)
	if err != nil {
		return nil, err
	}
	return s.CloneFromCheckpoint(ctx, CloneCheckpointRequest{
		SessionID:       checkpoint.SessionID,
		CheckpointID:    checkpoint.ID,
		TeamID:          checkpoint.TeamID,
		UserID:          req.UserID,
		AccessMode:      req.AccessMode,
		DefaultPosixUID: req.DefaultPosixUID,
		DefaultPosixGID: req.DefaultPosixGID,
	})
}

func (s *Service) Wake(ctx context.Context, req WakeRequest) (*WakeResult, error) {
	cursor, err := s.repo.GetSessionHarnessCursor(ctx, strings.TrimSpace(req.TeamID), strings.TrimSpace(req.SessionID), strings.TrimSpace(req.HarnessID))
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		return nil, err
	}
	afterSeq := int64(0)
	if cursor != nil {
		afterSeq = cursor.LastSeenSeq
	}
	events, err := s.GetEvents(ctx, req.TeamID, req.SessionID, EventRange{AfterSeq: afterSeq, Limit: req.Limit})
	if err != nil {
		return nil, err
	}
	return &WakeResult{Cursor: cursor, Events: events}, nil
}

func (s *Service) UpdateCursor(ctx context.Context, req UpdateCursorRequest) (*db.SessionHarnessCursor, error) {
	now := s.now()
	cursor := &db.SessionHarnessCursor{
		SessionID:   strings.TrimSpace(req.SessionID),
		TeamID:      strings.TrimSpace(req.TeamID),
		HarnessID:   strings.TrimSpace(req.HarnessID),
		LastSeenSeq: req.LastSeenSeq,
		State:       cloneRawPtr(req.State),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if cursor.SessionID == "" || cursor.TeamID == "" || cursor.HarnessID == "" {
		return nil, fmt.Errorf("%w: session_id, team_id and harness_id are required", ErrInvalidRequest)
	}
	return cursor, s.repo.UpsertSessionHarnessCursor(ctx, cursor)
}

func (s *Service) GetSmartestCheckpoint(ctx context.Context, teamID, sessionID string) (*db.SessionCheckpoint, error) {
	checkpoint, err := s.repo.GetSmartestSessionCheckpoint(ctx, strings.TrimSpace(teamID), strings.TrimSpace(sessionID))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, ErrCheckpointNotFound
		}
		return nil, err
	}
	return checkpoint, nil
}

func (s *Service) ListCheckpoints(ctx context.Context, teamID, sessionID string) ([]*db.SessionCheckpoint, error) {
	return s.repo.ListSessionCheckpoints(ctx, strings.TrimSpace(teamID), strings.TrimSpace(sessionID))
}

func (s *Service) CloneFromCheckpoint(ctx context.Context, req CloneCheckpointRequest) (*CloneCheckpointResult, error) {
	checkpoint, err := s.loadOwnedCheckpoint(ctx, req.TeamID, req.SessionID, req.CheckpointID)
	if err != nil {
		return nil, err
	}
	userID := strings.TrimSpace(req.UserID)
	if userID == "" {
		userID = checkpoint.UserID
	}
	volume, err := s.snapshot.ForkSnapshot(ctx, &ForkSnapshotBackendRequest{
		VolumeID:        checkpoint.VolumeID,
		SnapshotID:      checkpoint.SnapshotID,
		TeamID:          checkpoint.TeamID,
		UserID:          userID,
		AccessMode:      req.AccessMode,
		DefaultPosixUID: req.DefaultPosixUID,
		DefaultPosixGID: req.DefaultPosixGID,
	})
	if err != nil {
		return nil, err
	}
	return &CloneCheckpointResult{
		SourceSessionID: checkpoint.SessionID,
		CheckpointID:    checkpoint.ID,
		SnapshotID:      checkpoint.SnapshotID,
		SourceVolumeID:  checkpoint.VolumeID,
		Volume:          volume,
	}, nil
}

func (s *Service) RollbackToCheckpoint(ctx context.Context, req RollbackCheckpointRequest) (*db.SessionCheckpoint, error) {
	checkpoint, err := s.loadOwnedCheckpoint(ctx, req.TeamID, req.SessionID, req.CheckpointID)
	if err != nil {
		return nil, err
	}
	userID := strings.TrimSpace(req.UserID)
	if userID == "" {
		userID = checkpoint.UserID
	}
	if err := s.snapshot.RestoreSnapshot(ctx, &RestoreSnapshotBackendRequest{
		VolumeID:   checkpoint.VolumeID,
		SnapshotID: checkpoint.SnapshotID,
		TeamID:     checkpoint.TeamID,
		UserID:     userID,
	}); err != nil {
		return nil, err
	}
	return checkpoint, nil
}

func (s *Service) loadOwnedCheckpoint(ctx context.Context, teamID, sessionID, checkpointID string) (*db.SessionCheckpoint, error) {
	checkpoint, err := s.repo.GetSessionCheckpoint(ctx, strings.TrimSpace(checkpointID))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, ErrCheckpointNotFound
		}
		return nil, err
	}
	if checkpoint.TeamID != strings.TrimSpace(teamID) || checkpoint.SessionID != strings.TrimSpace(sessionID) {
		return nil, ErrCheckpointOwnership
	}
	return checkpoint, nil
}

func normalizeKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case db.SessionCheckpointKindSmartest:
		return db.SessionCheckpointKindSmartest
	default:
		return db.SessionCheckpointKindManual
	}
}

func snapshotName(sessionID, label string) string {
	name := "session-" + strings.TrimSpace(sessionID) + "-" + strings.TrimSpace(label)
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_' || r == '.':
			return r
		default:
			return '-'
		}
	}, name)
	if len(name) > 96 {
		return name[:96]
	}
	return name
}

func (s *Service) upsertRef(ctx context.Context, teamID, sessionID, refType, name, checkpointID string) error {
	now := s.now()
	return s.repo.UpsertSessionRef(ctx, &db.SessionRef{
		SessionID:    strings.TrimSpace(sessionID),
		TeamID:       strings.TrimSpace(teamID),
		RefType:      normalizeRefType(refType),
		Name:         strings.TrimSpace(name),
		CheckpointID: checkpointID,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
}

func normalizeRefType(refType string) string {
	switch strings.ToLower(strings.TrimSpace(refType)) {
	case db.SessionRefTypeTag:
		return db.SessionRefTypeTag
	default:
		return db.SessionRefTypeBranch
	}
}

func buildContextRecipe(entries []*db.SessionStageEntry) (*json.RawMessage, error) {
	type recipe struct {
		Version int                     `json:"version"`
		Stage   []*db.SessionStageEntry `json:"stage"`
	}
	payload, err := json.Marshal(recipe{Version: 1, Stage: entries})
	if err != nil {
		return nil, err
	}
	raw := json.RawMessage(payload)
	return &raw, nil
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	out := make([]byte, len(raw))
	copy(out, raw)
	return out
}

func cloneRawPtr(raw *json.RawMessage) *json.RawMessage {
	if raw == nil {
		return nil
	}
	cloned := cloneRaw(*raw)
	return &cloned
}
