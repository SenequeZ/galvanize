package ansible

import (
	"sync"
	"time"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var portBindingDBMu sync.Mutex

type portBindingRecord struct {
	DeploymentKey string    `gorm:"column:deployment_key;primaryKey"`
	ContainerPort string    `gorm:"column:container_port;primaryKey"`
	HostPort      int       `gorm:"column:host_port;not null;uniqueIndex"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (portBindingRecord) TableName() string {
	return "published_port_bindings"
}

func loadPortBindingsFromDB(dbPath, deploymentKey string) map[string]int {
	if dbPath == "" || deploymentKey == "" {
		return map[string]int{}
	}

	portBindingDBMu.Lock()
	defer portBindingDBMu.Unlock()

	db, err := openPortBindingDB(dbPath)
	if err != nil {
		return map[string]int{}
	}

	var records []portBindingRecord
	if err := db.Where("deployment_key = ?", deploymentKey).Find(&records).Error; err != nil {
		return map[string]int{}
	}

	bindings := map[string]int{}
	for _, record := range records {
		bindings[record.ContainerPort] = record.HostPort
	}
	return bindings
}

func ensureRandomPortBindingsInDB(dbPath, deploymentKey string, containerPorts []string) map[string]int {
	if dbPath == "" || deploymentKey == "" || len(containerPorts) == 0 {
		return map[string]int{}
	}

	portBindingDBMu.Lock()
	defer portBindingDBMu.Unlock()

	db, err := openPortBindingDB(dbPath)
	if err != nil {
		return map[string]int{}
	}

	result := map[string]int{}
	var existing []portBindingRecord
	if err := db.Where("deployment_key = ?", deploymentKey).Find(&existing).Error; err == nil {
		for _, rec := range existing {
			result[rec.ContainerPort] = rec.HostPort
		}
	}

	for _, containerPort := range containerPorts {
		if _, ok := result[containerPort]; ok {
			continue
		}

		hostPort := reserveRandomHostPort(db, deploymentKey, containerPort)
		if hostPort > 0 {
			result[containerPort] = hostPort
		}
	}

	return result
}

func reserveRandomHostPort(db *gorm.DB, deploymentKey, containerPort string) int {
	if db == nil || deploymentKey == "" || containerPort == "" {
		return 0
	}

	// Fast path: another worker may have already inserted this mapping.
	var existing portBindingRecord
	if err := db.First(&existing, "deployment_key = ? AND container_port = ?", deploymentKey, containerPort).Error; err == nil {
		return existing.HostPort
	}

	for range 128 {
		hostPort := randomHostPortFromDB(db)
		if hostPort == 0 {
			continue
		}
		record := portBindingRecord{
			DeploymentKey: deploymentKey,
			ContainerPort: containerPort,
			HostPort:      hostPort,
		}
		res := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
		if res.Error != nil {
			continue
		}
		if res.RowsAffected == 1 {
			return hostPort
		}

		// If insert was ignored, check whether our target mapping now exists.
		var mapped portBindingRecord
		if err := db.First(&mapped, "deployment_key = ? AND container_port = ?", deploymentKey, containerPort).Error; err == nil {
			return mapped.HostPort
		}
	}

	return 0
}

func randomHostPortFromDB(db *gorm.DB) int {
	if db == nil {
		return 0
	}
	const minPort = 20000
	const maxPort = 60999
	const rangeSize = maxPort - minPort + 1

	var hostPort int
	if err := db.Raw("SELECT CAST((ABS(RANDOM()) % ?) + ? AS INTEGER)", rangeSize, minPort).Scan(&hostPort).Error; err != nil {
		return 0
	}
	if hostPort < minPort || hostPort > maxPort {
		return 0
	}
	return hostPort
}

func savePortBindingsToDB(dbPath, deploymentKey string, bindings map[string]int) {
	if dbPath == "" || deploymentKey == "" || len(bindings) == 0 {
		return
	}

	portBindingDBMu.Lock()
	defer portBindingDBMu.Unlock()

	db, err := openPortBindingDB(dbPath)
	if err != nil {
		return
	}

	for containerPort, hostPort := range bindings {
		record := portBindingRecord{
			DeploymentKey: deploymentKey,
			ContainerPort: containerPort,
			HostPort:      hostPort,
		}
		_ = db.Save(&record).Error
	}
}

func clearPortBindingsFromDB(dbPath, deploymentKey string) {
	if dbPath == "" || deploymentKey == "" {
		return
	}

	portBindingDBMu.Lock()
	defer portBindingDBMu.Unlock()

	db, err := openPortBindingDB(dbPath)
	if err != nil {
		return
	}

	_ = db.Delete(&portBindingRecord{}, "deployment_key = ?", deploymentKey).Error
}

// CleanupStalePortBindings removes port binding rows that no longer map to a
// non-deleted deployment entry.
func CleanupStalePortBindings(dbPath string) {
	if dbPath == "" {
		return
	}

	portBindingDBMu.Lock()
	defer portBindingDBMu.Unlock()

	db, err := openPortBindingDB(dbPath)
	if err != nil {
		zap.S().Warnf("Failed to open DB for stale port binding cleanup: %v", err)
		return
	}

	res := db.Exec(`
		DELETE FROM published_port_bindings
		WHERE deployment_key NOT IN (
			SELECT
				category || '/' || challenge_name || ':' || COALESCE(team_id, '')
			FROM deployments
			WHERE deleted_at IS NULL
		)
	`)
	if res.Error != nil {
		zap.S().Warnf("Failed to cleanup stale port bindings: %v", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		zap.S().Infof("Removed %d stale port binding rows", res.RowsAffected)
	}
}

func openPortBindingDB(dbPath string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&portBindingRecord{}); err != nil {
		return nil, err
	}
	return db, nil
}
