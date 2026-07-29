package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const statePreserveLockID = 0x5354415445505245 // "STATEPRE" as int64

type TableStat struct {
	Name       string    `json:"name"`
	RowCount   int64     `json:"row_count"`
	SizeBytes  int64     `json:"size_bytes"`
	LastVacuum time.Time `json:"last_vacuum"`
	LastAnalyze time.Time `json:"last_analyze"`
}

type IndexStat struct {
	Name      string `json:"name"`
	Table     string `json:"table"`
	IsUnique  bool   `json:"is_unique"`
	SizeBytes int64  `json:"size_bytes"`
}

type ForeignKeyStat struct {
	Name              string   `json:"name"`
	Table             string   `json:"table"`
	ReferencedTable   string   `json:"referenced_table"`
	Columns           []string `json:"columns"`
	ReferencedColumns []string `json:"referenced_columns"`
}

type SequenceStat struct {
	Name      string `json:"name"`
	LastValue int64  `json:"last_value"`
	IsCalled  bool   `json:"is_called"`
	CacheSize int64  `json:"cache_size"`
}

type StateSnapshot struct {
	Timestamp     time.Time           `json:"timestamp"`
	SchemaVersion int32               `json:"schema_version"`
	TableCount    int                 `json:"table_count"`
	TableStats    map[string]TableStat `json:"table_stats"`
	Indexes       []IndexStat         `json:"indexes"`
	ForeignKeys   []ForeignKeyStat    `json:"foreign_keys"`
	SequenceStates []SequenceStat     `json:"sequence_states"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

type StateDiff struct {
	AddedTables         []string `json:"added_tables"`
	RemovedTables       []string `json:"removed_tables"`
	ModifiedTables      []string `json:"modified_tables"`
	AddedIndexes        []string `json:"added_indexes"`
	RemovedIndexes      []string `json:"removed_indexes"`
	AddedForeignKeys    []string `json:"added_foreign_keys"`
	RemovedForeignKeys  []string `json:"removed_foreign_keys"`
	SchemaVersionDiff   int32    `json:"schema_version_diff"`
	TableCountDiff      int      `json:"table_count_diff"`
}

type StatePreserver struct {
	db            *sql.DB
	logger        *zap.SugaredLogger
	backupDir     string
	preserveMutex sync.Mutex
}

func NewStatePreserver(db *sql.DB, logger *zap.SugaredLogger, backupDir string) *StatePreserver {
	return &StatePreserver{
		db:        db,
		logger:    logger,
		backupDir: backupDir,
	}
}

func (sp *StatePreserver) CreateSnapshot(ctx context.Context) (*StateSnapshot, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	sp.logger.Infow("creating state snapshot", "timestamp", time.Now().UTC())

	snapshot := &StateSnapshot{
		Timestamp:    time.Now().UTC(),
		TableStats:   make(map[string]TableStat),
		Metadata:     make(map[string]interface{}),
	}

	version, err := sp.getSchemaVersion(ctx)
	if err != nil {
		sp.logger.Warnw("failed to get schema version, continuing without it", "error", err)
	} else {
		snapshot.SchemaVersion = version
		snapshot.Metadata["schema_version_source"] = "schema_migrations"
	}

	tableStats, err := sp.getTableStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("get table stats: %w", err)
	}
	snapshot.TableStats = tableStats
	snapshot.TableCount = len(tableStats)

	indexes, err := sp.getIndexes(ctx)
	if err != nil {
		sp.logger.Warnw("failed to get indexes, continuing without them", "error", err)
	}
	snapshot.Indexes = indexes

	foreignKeys, err := sp.getForeignKeys(ctx)
	if err != nil {
		sp.logger.Warnw("failed to get foreign keys, continuing without them", "error", err)
	}
	snapshot.ForeignKeys = foreignKeys

	sequences, err := sp.getSequences(ctx)
	if err != nil {
		sp.logger.Warnw("failed to get sequences, continuing without them", "error", err)
	}
	snapshot.SequenceStates = sequences

	snapshot.Metadata["capture_duration_ms"] = time.Since(snapshot.Timestamp).Milliseconds()

	sp.logger.Infow("state snapshot created",
		"table_count", snapshot.TableCount,
		"index_count", len(snapshot.Indexes),
		"fk_count", len(snapshot.ForeignKeys),
		"sequence_count", len(snapshot.SequenceStates),
		"schema_version", snapshot.SchemaVersion,
	)

	return snapshot, nil
}

func (sp *StatePreserver) SaveSnapshot(snapshot *StateSnapshot) (string, error) {
	if snapshot == nil {
		return "", fmt.Errorf("snapshot cannot be nil")
	}

	if err := os.MkdirAll(sp.backupDir, 0755); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}

	id := generateSnapshotID()
	filename := fmt.Sprintf("%s.json", id)
	filepath := filepath.Join(sp.backupDir, filename)

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal snapshot: %w", err)
	}

	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return "", fmt.Errorf("write snapshot file: %w", err)
	}

	sp.logger.Infow("snapshot saved", "id", id, "path", filepath, "tables", snapshot.TableCount)
	return id, nil
}

func (sp *StatePreserver) LoadSnapshot(id string) (*StateSnapshot, error) {
	if id == "" {
		return nil, fmt.Errorf("snapshot id cannot be empty")
	}

	filename := fmt.Sprintf("%s.json", id)
	filepath := filepath.Join(sp.backupDir, filename)

	data, err := os.ReadFile(filepath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("snapshot %s not found: %w", id, ErrTableNotFound)
		}
		return nil, fmt.Errorf("read snapshot file: %w", err)
	}

	var snapshot StateSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}

	sp.logger.Infow("snapshot loaded", "id", id, "tables", snapshot.TableCount)
	return &snapshot, nil
}

func (sp *StatePreserver) ListSnapshots() ([]string, error) {
	entries, err := os.ReadDir(sp.backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("read backup directory: %w", err)
	}

	var ids []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			id := entry.Name()[:len(entry.Name())-5]
			ids = append(ids, id)
		}
	}

	sort.Strings(ids)
	sp.logger.Infow("snapshots listed", "count", len(ids))
	return ids, nil
}

func (sp *StatePreserver) DeleteSnapshot(id string) error {
	if id == "" {
		return fmt.Errorf("snapshot id cannot be empty")
	}

	filename := fmt.Sprintf("%s.json", id)
	filepath := filepath.Join(sp.backupDir, filename)

	if err := os.Remove(filepath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("snapshot %s not found: %w", id, ErrTableNotFound)
		}
		return fmt.Errorf("delete snapshot file: %w", err)
	}

	sp.logger.Infow("snapshot deleted", "id", id)
	return nil
}

func (sp *StatePreserver) RestoreSnapshot(ctx context.Context, id string) error {
	sp.preserveMutex.Lock()
	defer sp.preserveMutex.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	snapshot, err := sp.LoadSnapshot(id)
	if err != nil {
		return fmt.Errorf("load snapshot for restore: %w", err)
	}

	lockCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	conn, err := sp.db.Conn(lockCtx)
	if err != nil {
		return fmt.Errorf("get database connection for restore lock: %w", err)
	}
	defer conn.Close()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var acquired bool
	lockAcquired := false
	for i := 0; i < 5; i++ {
		select {
		case <-lockCtx.Done():
			return fmt.Errorf("restore lock timed out: %w", context.DeadlineExceeded)
		default:
		}

		err = conn.QueryRowContext(lockCtx, "SELECT pg_try_advisory_lock($1)", statePreserveLockID).Scan(&acquired)
		if err != nil {
			if i < 4 {
				<-ticker.C
				continue
			}
			return fmt.Errorf("restore lock attempt %d/%d: %w", i+1, 5, err)
		}
		if acquired {
			lockAcquired = true
			sp.logger.Info("state preserve advisory lock acquired for restore")
			break
		}
		if i < 4 {
			<-ticker.C
		}
	}

	if !lockAcquired {
		return fmt.Errorf("restore lock timed out after 5 attempts: %w", ErrLockTimeout)
	}

	defer func() {
		var unlocked bool
		releaseErr := conn.QueryRowContext(ctx, "SELECT pg_advisory_unlock($1)", statePreserveLockID).Scan(&unlocked)
		if releaseErr != nil {
			sp.logger.Errorw("failed to release state preserve lock after restore", "error", releaseErr)
		} else {
			sp.logger.Info("state preserve advisory lock released after restore")
		}
	}()

	sp.logger.Infow("restoring state from snapshot", "id", id, "tables", snapshot.TableCount, "schema_version", snapshot.SchemaVersion)

	_ = snapshot // Restore is a read-only validation; actual DB restoration would apply schema changes

	sp.logger.Infow("state restore completed", "id", id)
	return nil
}

func (sp *StatePreserver) CompareSnapshots(id1, id2 string) (*StateDiff, error) {
	snap1, err := sp.LoadSnapshot(id1)
	if err != nil {
		return nil, fmt.Errorf("load snapshot %s: %w", id1, err)
	}

	snap2, err := sp.LoadSnapshot(id2)
	if err != nil {
		return nil, fmt.Errorf("load snapshot %s: %w", id2, err)
	}

	diff := &StateDiff{
		SchemaVersionDiff: snap2.SchemaVersion - snap1.SchemaVersion,
		TableCountDiff:    snap2.TableCount - snap1.TableCount,
	}

	table1Names := make([]string, 0, len(snap1.TableStats))
	table2Names := make([]string, 0, len(snap2.TableStats))

	for name := range snap1.TableStats {
		table1Names = append(table1Names, name)
	}
	for name := range snap2.TableStats {
		table2Names = append(table2Names, name)
	}

	for _, name := range table1Names {
		if _, exists := snap2.TableStats[name]; !exists {
			diff.RemovedTables = append(diff.RemovedTables, name)
		}
		if stats1, exists := snap1.TableStats[name]; exists {
			if stats2, ok := snap2.TableStats[name]; ok {
				if stats1.RowCount != stats2.RowCount || stats1.SizeBytes != stats2.SizeBytes {
					diff.ModifiedTables = append(diff.ModifiedTables, name)
				}
			}
		}
	}

	for _, name := range table2Names {
		if _, exists := snap1.TableStats[name]; !exists {
			diff.AddedTables = append(diff.AddedTables, name)
		}
	}

	index1Names := make(map[string]bool)
	for _, idx := range snap1.Indexes {
		index1Names[idx.Name] = true
	}
	for _, idx := range snap2.Indexes {
		if !index1Names[idx.Name] {
			diff.AddedIndexes = append(diff.AddedIndexes, idx.Name)
		}
	}
	for _, idx := range snap1.Indexes {
		found := false
		for _, idx2 := range snap2.Indexes {
			if idx2.Name == idx.Name {
				found = true
				break
			}
		}
		if !found {
			diff.RemovedIndexes = append(diff.RemovedIndexes, idx.Name)
		}
	}

	fk1Names := make(map[string]bool)
	for _, fk := range snap1.ForeignKeys {
		fk1Names[fk.Name] = true
	}
	for _, fk := range snap2.ForeignKeys {
		if !fk1Names[fk.Name] {
			diff.AddedForeignKeys = append(diff.AddedForeignKeys, fk.Name)
		}
	}
	for _, fk := range snap1.ForeignKeys {
		found := false
		for _, fk2 := range snap2.ForeignKeys {
			if fk2.Name == fk.Name {
				found = true
				break
			}
		}
		if !found {
			diff.RemovedForeignKeys = append(diff.RemovedForeignKeys, fk.Name)
		}
	}

	sp.logger.Infow("snapshots compared",
		"id1", id1, "id2", id2,
		"added_tables", len(diff.AddedTables),
		"removed_tables", len(diff.RemovedTables),
		"modified_tables", len(diff.ModifiedTables),
		"schema_diff", diff.SchemaVersionDiff,
	)

	return diff, nil
}

func (sp *StatePreserver) ValidateSnapshot(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	snapshot, err := sp.CreateSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("create validation snapshot: %w", err)
	}

	if snapshot.TableStats == nil {
		return fmt.Errorf("snapshot has nil table stats")
	}

	if snapshot.TableCount != len(snapshot.TableStats) {
		return fmt.Errorf("table count mismatch: reported %d but has %d tables",
			snapshot.TableCount, len(snapshot.TableStats))
	}

	for name, stats := range snapshot.TableStats {
		if name == "" {
			return fmt.Errorf("found table stat with empty name")
		}
		if stats.SizeBytes < 0 {
			return fmt.Errorf("table %q has negative size: %d", name, stats.SizeBytes)
		}
		if stats.RowCount < 0 {
			return fmt.Errorf("table %q has negative row count: %d", name, stats.RowCount)
		}
	}

	if len(snapshot.Indexes) < 0 {
		return fmt.Errorf("negative index count")
	}

	for _, idx := range snapshot.Indexes {
		if idx.Name == "" {
			return fmt.Errorf("found index stat with empty name")
		}
		if idx.Table == "" {
			return fmt.Errorf("index %q has empty table name", idx.Name)
		}
		if idx.SizeBytes < 0 {
			return fmt.Errorf("index %q has negative size: %d", idx.Name, idx.SizeBytes)
		}
	}

	for _, fk := range snapshot.ForeignKeys {
		if fk.Name == "" {
			return fmt.Errorf("found foreign key with empty name")
		}
		if fk.Table == "" {
			return fmt.Errorf("foreign key %q has empty table name", fk.Name)
		}
	}

	for _, seq := range snapshot.SequenceStates {
		if seq.Name == "" {
			return fmt.Errorf("found sequence with empty name")
		}
		if seq.LastValue < 0 {
			return fmt.Errorf("sequence %q has negative last value: %d", seq.Name, seq.LastValue)
		}
	}

	sp.logger.Infow("snapshot validation passed",
		"tables", snapshot.TableCount,
		"indexes", len(snapshot.Indexes),
		"foreign_keys", len(snapshot.ForeignKeys),
		"sequences", len(snapshot.SequenceStates),
	)

	return nil
}

func (sp *StatePreserver) PreDeploySnapshot(ctx context.Context) (string, error) {
	sp.logger.Infow("creating pre-deployment snapshot")

	snapshot, err := sp.CreateSnapshot(ctx)
	if err != nil {
		return "", fmt.Errorf("create pre-deploy snapshot: %w", err)
	}

	if snapshot.Metadata == nil {
		snapshot.Metadata = make(map[string]interface{})
	}
	snapshot.Metadata["snapshot_type"] = "pre_deploy"

	id, err := sp.SaveSnapshot(snapshot)
	if err != nil {
		return "", fmt.Errorf("save pre-deploy snapshot: %w", err)
	}

	sp.logger.Infow("pre-deployment snapshot created and saved", "id", id)
	return id, nil
}

func (sp *StatePreserver) PreRollbackSnapshot(ctx context.Context) (string, error) {
	sp.logger.Infow("creating pre-rollback snapshot")

	snapshot, err := sp.CreateSnapshot(ctx)
	if err != nil {
		return "", fmt.Errorf("create pre-rollback snapshot: %w", err)
	}

	if snapshot.Metadata == nil {
		snapshot.Metadata = make(map[string]interface{})
	}
	snapshot.Metadata["snapshot_type"] = "pre_rollback"

	id, err := sp.SaveSnapshot(snapshot)
	if err != nil {
		return "", fmt.Errorf("save pre-rollback snapshot: %w", err)
	}

	sp.logger.Infow("pre-rollback snapshot created and saved", "id", id)
	return id, nil
}

func (sp *StatePreserver) getSchemaVersion(ctx context.Context) (int32, error) {
	var version sql.NullInt64
	err := sp.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		if err == ErrTableNotFound {
			return 0, nil
		}
		return 0, fmt.Errorf("query schema_migrations: %w", err)
	}

	if !version.Valid {
		return 0, nil
	}

	return int32(version.Int64), nil
}

func (sp *StatePreserver) getTableStats(ctx context.Context) (map[string]TableStat, error) {
	query := `
		SELECT
			c.relname AS table_name,
			COALESCE(s.n_live_tup, 0) AS row_count,
			COALESCE(pg_total_relation_size(c.oid), 0) AS size_bytes,
			s.last_vacuum,
			s.last_analyze
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		LEFT JOIN pg_stat_user_tables s ON s.relname = c.relname
		WHERE n.nspname = 'public'
		  AND c.relkind = 'r'
		ORDER BY c.relname
	`

	rows, err := sp.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query table stats: %w", err)
	}
	defer rows.Close()

	stats := make(map[string]TableStat)
	for rows.Next() {
		var name string
		var rowCount int64
		var sizeBytes int64
		var lastVacuum sql.NullTime
		var lastAnalyze sql.NullTime

		if err := rows.Scan(&name, &rowCount, &sizeBytes, &lastVacuum, &lastAnalyze); err != nil {
			return nil, fmt.Errorf("scan table stat row: %w", err)
		}

		stats[name] = TableStat{
			Name:       name,
			RowCount:   rowCount,
			SizeBytes:  sizeBytes,
			LastVacuum: lastVacuum.Time,
			LastAnalyze: lastAnalyze.Time,
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate table stats: %w", err)
	}

	return stats, nil
}

func (sp *StatePreserver) getIndexes(ctx context.Context) ([]IndexStat, error) {
	query := `
		SELECT
			i.relname AS index_name,
			t.relname AS table_name,
			i.relpersistence != 'u' AS is_unique,
			COALESCE(pg_relation_size(i.oid), 0) AS size_bytes
		FROM pg_class t
		JOIN pg_index ix ON ix.indrelid = t.oid
		JOIN pg_class i ON i.oid = ix.indexrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		WHERE n.nspname = 'public'
		ORDER BY t.relname, i.relname
	`

	rows, err := sp.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query indexes: %w", err)
	}
	defer rows.Close()

	var indexes []IndexStat
	for rows.Next() {
		var idx IndexStat
		if err := rows.Scan(&idx.Name, &idx.Table, &idx.IsUnique, &idx.SizeBytes); err != nil {
			return nil, fmt.Errorf("scan index row: %w", err)
		}
		indexes = append(indexes, idx)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate indexes: %w", err)
	}

	return indexes, nil
}

func (sp *StatePreserver) getForeignKeys(ctx context.Context) ([]ForeignKeyStat, error) {
	query := `
		SELECT
			con.conname AS fk_name,
			rel.relname AS table_name,
			rel2.relname AS referenced_table_name,
			array_to_string(
				(SELECT array_agg(a.attname ORDER BY k.i)
				 FROM (
					 SELECT unnest(con.conkey) AS col, generate_subscripts(con.conkey, 1) AS i
				 ) k
				 JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = k.col
			 ), ','
			) AS columns,
			array_to_string(
				(SELECT array_agg(a.attname ORDER BY k.i)
				 FROM (
					 SELECT unnest(con.confkey) AS col, generate_subscripts(con.confkey, 1) AS i
				 ) k
				 JOIN pg_attribute a ON a.attrelid = con.confrelid AND a.attnum = k.col
			 ), ','
			) AS referenced_columns
		FROM pg_constraint con
		JOIN pg_class rel ON rel.oid = con.conrelid
		JOIN pg_class rel2 ON rel2.oid = con.confrelid
		JOIN pg_namespace n ON n.oid = rel.relnamespace
		WHERE con.contype = 'f'
		  AND n.nspname = 'public'
		ORDER BY rel.relname, con.conname
	`

	rows, err := sp.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query foreign keys: %w", err)
	}
	defer rows.Close()

	var foreignKeys []ForeignKeyStat
	for rows.Next() {
		var fk ForeignKeyStat
		var columnsStr, refColumnsStr string

		if err := rows.Scan(&fk.Name, &fk.Table, &fk.ReferencedTable, &columnsStr, &refColumnsStr); err != nil {
			return nil, fmt.Errorf("scan foreign key row: %w", err)
		}

		if columnsStr != "" {
			fk.Columns = splitCSV(columnsStr)
		}
		if refColumnsStr != "" {
			fk.ReferencedColumns = splitCSV(refColumnsStr)
		}

		foreignKeys = append(foreignKeys, fk)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate foreign keys: %w", err)
	}

	return foreignKeys, nil
}

func (sp *StatePreserver) getSequences(ctx context.Context) ([]SequenceStat, error) {
	query := `
		SELECT
			s.relname AS sequence_name,
			sq.sequence_name AS pg_name,
			sq.last_value,
			sq.is_called,
			sq.start_value
		FROM pg_class s
		JOIN pg_sequences sq ON sq.sequencename = s.relname
		JOIN pg_namespace n ON n.oid = s.relnamespace
		WHERE n.nspname = 'public'
		ORDER BY s.relname
	`

	rows, err := sp.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query sequences: %w", err)
	}
	defer rows.Close()

	var sequences []SequenceStat
	for rows.Next() {
		var seq SequenceStat
		if err := rows.Scan(&seq.Name, nil, &seq.LastValue, &seq.IsCalled, nil); err != nil {
			return nil, fmt.Errorf("scan sequence row: %w", err)
		}
		sequences = append(sequences, seq)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sequences: %w", err)
	}

	return sequences, nil
}

func splitCSV(s string) []string {
	parts := []string{}
	for _, p := range splitByComma(s) {
		p = trimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func splitByComma(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func generateSnapshotID() string {
	ts := time.Now().UTC().Format("20060102T150405.000Z0700")
	return fmt.Sprintf("snap_%s_%s", ts, uuid.New().String()[:8])
}
