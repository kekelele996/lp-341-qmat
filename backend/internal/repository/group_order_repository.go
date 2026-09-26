package repository

import (
	"errors"
	"time"

	"github.com/blueship581/gbcheckup/internal/constants"
	"github.com/blueship581/gbcheckup/internal/model"
	"github.com/blueship581/gbcheckup/internal/util"
	"gorm.io/gorm"
)

// GroupOrderRepository 团检订单仓储。
type GroupOrderRepository struct{ db *gorm.DB }

// NewGroupOrderRepository 构造团检订单仓储。
func NewGroupOrderRepository(db *gorm.DB) *GroupOrderRepository { return &GroupOrderRepository{db: db} }

// WithTx 使用事务连接构造仓储。
func (r *GroupOrderRepository) WithTx(tx *gorm.DB) *GroupOrderRepository {
	return &GroupOrderRepository{db: tx}
}

// Transaction 在事务内执行 fn，任一步返回 error 则整体回滚。
func (r *GroupOrderRepository) Transaction(fn func(tx *gorm.DB) error) error {
	return r.db.Transaction(fn)
}

func (r *GroupOrderRepository) Create(o *model.GroupOrder) error { return r.db.Create(o).Error }

func (r *GroupOrderRepository) List(page, pageSize int) ([]model.GroupOrder, int64, error) {
	var total int64
	if err := r.db.Model(&model.GroupOrder{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.GroupOrder
	err := r.db.Preload("Enterprise").Preload("Package").Order("id desc").Offset((page-1)*pageSize).Limit(pageSize).Find(&items).Error
	return items, total, err
}

func (r *GroupOrderRepository) FindByID(id uint) (*model.GroupOrder, error) {
	var o model.GroupOrder
	if err := r.db.Preload("Enterprise").Preload("Package").First(&o, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, util.ErrNotFound
		}
		return nil, err
	}
	return &o, nil
}

func (r *GroupOrderRepository) Update(o *model.GroupOrder) error { return r.db.Save(o).Error }

// readinessSelect 按 企业+套餐 归集已登记体检人并统计报告已发布人数。
const readinessSelect = `e.enterprise_id AS enterprise_id, reg.package_id AS package_id,
	COUNT(DISTINCT e.id) AS registered_count,
	COUNT(DISTINCT CASE WHEN rep.status = '` + constants.ReportPublished + `' THEN e.id END) AS ready_count`

// readinessJoins 归集口径：体检人归属企业（examinees.enterprise_id）且登记了订单套餐（registrations.package_id）。
const readinessJoins = `JOIN examinees e ON e.id = reg.examinee_id
	LEFT JOIN reports rep ON rep.registration_id = reg.id`

// ReadinessSummary 全量 企业+套餐 维度的报告就绪统计（订单列表批量挂载进度用）。
func (r *GroupOrderRepository) ReadinessSummary() (map[model.OrderKey]model.OrderReadiness, error) {
	var rows []model.OrderReadiness
	err := r.db.Table("registrations reg").
		Select(readinessSelect).
		Joins(readinessJoins).
		Where("e.enterprise_id IS NOT NULL").
		Group("e.enterprise_id, reg.package_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	m := make(map[model.OrderKey]model.OrderReadiness, len(rows))
	for _, row := range rows {
		m[model.OrderKey{EnterpriseID: row.EnterpriseID, PackageID: row.PackageID}] = row
	}
	return m, nil
}

// Readiness 单个 企业+套餐 维度的报告就绪统计。
func (r *GroupOrderRepository) Readiness(enterpriseID, packageID uint) (*model.OrderReadiness, error) {
	row := model.OrderReadiness{EnterpriseID: enterpriseID, PackageID: packageID}
	err := r.db.Table("registrations reg").
		Select(readinessSelect).
		Joins(readinessJoins).
		Where("e.enterprise_id = ? AND reg.package_id = ?", enterpriseID, packageID).
		Group("e.enterprise_id, reg.package_id").
		Scan(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// ListMembers 归集订单（企业+套餐）下的已登记体检人及其报告状态。
func (r *GroupOrderRepository) ListMembers(enterpriseID, packageID uint) ([]model.GroupOrderMember, error) {
	var members []model.GroupOrderMember
	err := r.db.Table("registrations reg").
		Select(`e.id AS examinee_id, e.name AS examinee_name, e.id_card_no,
			reg.id AS registration_id, reg.guide_no, reg.status AS registration_status,
			rep.id AS report_id, COALESCE(rep.report_no, '') AS report_no, COALESCE(rep.status, '') AS report_status`).
		Joins(readinessJoins).
		Where("e.enterprise_id = ? AND reg.package_id = ?", enterpriseID, packageID).
		Order("reg.id").
		Scan(&members).Error
	return members, err
}

// MarkDelivered 一次性更新订单状态、交付状态与交付时间；仅当尚未交付时生效。
// 返回是否由本次调用完成交付（并发/重复交付时返回 false，调用方按幂等成功处理）。
func (r *GroupOrderRepository) MarkDelivered(id uint, deliveredAt time.Time) (bool, error) {
	res := r.db.Model(&model.GroupOrder{}).
		Where("id = ? AND report_delivery_status <> ?", id, constants.GroupOrderDeliveryDelivered).
		Updates(map[string]any{
			"status":                 constants.GroupOrderDone,
			"report_delivery_status": constants.GroupOrderDeliveryDelivered,
			"delivered_at":           deliveredAt,
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}
