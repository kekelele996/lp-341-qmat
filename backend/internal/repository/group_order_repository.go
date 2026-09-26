package repository

import (
	"errors"
	"fmt"
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

func (r *GroupOrderRepository) Create(o *model.GroupOrder) error { return r.db.Create(o).Error }

func (r *GroupOrderRepository) List(page, pageSize int) ([]model.GroupOrder, int64, error) {
	var total int64
	if err := r.db.Model(&model.GroupOrder{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.GroupOrder
	err := r.db.Preload("Enterprise").Preload("Package").Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error
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

// ProgressByIDs 按企业和套餐归集已登记体检人，并统计已发布报告人数。
func (r *GroupOrderRepository) ProgressByIDs(ids []uint) (map[uint]model.GroupOrderProgress, error) {
	result := make(map[uint]model.GroupOrderProgress, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	var rows []struct {
		OrderID       uint
		RequiredCount int
		ReadyCount    int
	}
	err := r.db.Model(&model.GroupOrder{}).
		Select(`group_orders.id AS order_id,
			(SELECT COUNT(DISTINCT examinees.id)
			 FROM examinees
			 JOIN registrations ON registrations.examinee_id = examinees.id
			 WHERE examinees.enterprise_id = group_orders.enterprise_id
			   AND registrations.package_id = group_orders.package_id) AS required_count,
			(SELECT COUNT(DISTINCT examinees.id)
			 FROM examinees
			 JOIN registrations ON registrations.examinee_id = examinees.id
			 JOIN reports ON reports.registration_id = registrations.id AND reports.status = 'published'
			 WHERE examinees.enterprise_id = group_orders.enterprise_id
			   AND registrations.package_id = group_orders.package_id) AS ready_count`).
		Where("group_orders.id IN ?", ids).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.OrderID] = model.GroupOrderProgress{
			RequiredCount:      row.RequiredCount,
			ReadyCount:         row.ReadyCount,
			PendingReportCount: max(row.RequiredCount-row.ReadyCount, 0),
		}
	}
	return result, nil
}

// ExamineesByOrder 返回订单对应企业与套餐下的已登记体检人和当前报告状态。
func (r *GroupOrderRepository) ExamineesByOrder(order *model.GroupOrder) ([]model.GroupOrderExaminee, error) {
	var rows []model.GroupOrderExaminee
	readySQL := fmt.Sprintf("CASE WHEN reports.status = '%s' THEN 1 ELSE 0 END", constants.ReportPublished)
	err := r.db.Table("examinees").
		Select(`examinees.id AS examinee_id, examinees.name AS name, examinees.phone AS phone,
			examinees.gender AS gender, examinees.age AS age,
			registrations.id AS registration_id, registrations.guide_no AS guide_no,
			reports.id AS report_id, reports.report_no AS report_no,
			COALESCE(reports.status, '') AS report_status,
			`+readySQL+` AS ready`).
		Joins(fmt.Sprintf("JOIN registrations ON registrations.examinee_id = examinees.id AND registrations.package_id = %d", order.PackageID)).
		Joins("LEFT JOIN reports ON reports.registration_id = registrations.id").
		Where("examinees.enterprise_id = ?", order.EnterpriseID).
		Order("examinees.id ASC, CASE WHEN reports.status = 'published' THEN 0 ELSE 1 END, registrations.id DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	// 同一人体检/重登记时只保留最新登记记录；若已有发布报告，优先展示发布记录。
	seen := make(map[uint]bool, len(rows))
	examinees := make([]model.GroupOrderExaminee, 0, len(rows))
	for _, row := range rows {
		if seen[row.ExamineeID] {
			continue
		}
		seen[row.ExamineeID] = true
		examinees = append(examinees, row)
	}
	return examinees, nil
}

// MarkDelivered 仅当订单尚未交付时，在同一条更新中写入订单状态、交付状态与交付时间。
func (r *GroupOrderRepository) MarkDelivered(id uint, deliveredAt time.Time) (bool, error) {
	result := r.db.Model(&model.GroupOrder{}).
		Where("id = ? AND report_delivery_status <> ?", id, constants.ReportDeliveryDelivered).
		Updates(map[string]any{
			"status":                 constants.GroupOrderDone,
			"report_delivery_status": constants.ReportDeliveryDelivered,
			"delivered_at":           deliveredAt,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}
