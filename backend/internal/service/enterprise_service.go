package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/blueship581/gbcheckup/internal/constants"
	"github.com/blueship581/gbcheckup/internal/dto"
	"github.com/blueship581/gbcheckup/internal/model"
	"github.com/blueship581/gbcheckup/internal/repository"
	"github.com/blueship581/gbcheckup/internal/util"
)

// EnterpriseService 团检企业与订单服务。
type EnterpriseService struct {
	entRepo   *repository.EnterpriseRepository
	orderRepo *repository.GroupOrderRepository
	pkgRepo   *repository.PackageRepository
	log       *slog.Logger
}

// NewEnterpriseService 构造团检服务。
func NewEnterpriseService(entRepo *repository.EnterpriseRepository, orderRepo *repository.GroupOrderRepository, pkgRepo *repository.PackageRepository, log *slog.Logger) *EnterpriseService {
	return &EnterpriseService{entRepo: entRepo, orderRepo: orderRepo, pkgRepo: pkgRepo, log: log}
}

// CreateEnterprise 创建企业。
func (s *EnterpriseService) CreateEnterprise(ctx context.Context, e *model.Enterprise) (*model.Enterprise, error) {
	if err := s.entRepo.Create(e); err != nil {
		return nil, util.LogError(s.log, constants.LOG_ENTERPRISE_CREATED, fmt.Errorf("create enterprise: %w", err))
	}
	s.log.InfoContext(ctx, constants.LOG_ENTERPRISE_CREATED, "enterprise_id", e.ID)
	return e, nil
}

// ListEnterprises 企业列表。
func (s *EnterpriseService) ListEnterprises(ctx context.Context, page, pageSize int) ([]model.Enterprise, int64, error) {
	return s.entRepo.List(page, pageSize)
}

// CreateOrder 创建团检订单。
func (s *EnterpriseService) CreateOrder(ctx context.Context, enterpriseID, packageID uint, count int) (*model.GroupOrder, error) {
	if _, err := s.entRepo.FindByID(enterpriseID); err != nil {
		return nil, util.NotFoundError("团检企业（Enterprise）不存在", err)
	}
	if _, err := s.pkgRepo.FindByID(packageID); err != nil {
		return nil, util.NotFoundError(constants.MsgPackageNotFound, err)
	}
	order := &model.GroupOrder{EnterpriseID: enterpriseID, PackageID: packageID, ExamineeCount: count, Status: constants.GroupOrderPending, ReportDeliveryStatus: constants.ReportDeliveryPending}
	if err := s.orderRepo.Create(order); err != nil {
		return nil, util.LogError(s.log, constants.LOG_GROUP_ORDER_CREATED, fmt.Errorf("create group order: %w", err))
	}
	s.log.InfoContext(ctx, constants.LOG_GROUP_ORDER_CREATED, "order_id", order.ID)
	return order, nil
}

// ListOrders 团检订单列表，并附上按企业和套餐归集的报告进度。
func (s *EnterpriseService) ListOrders(ctx context.Context, page, pageSize int) ([]dto.GroupOrderListItem, int64, error) {
	orders, total, err := s.orderRepo.List(page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	ids := make([]uint, 0, len(orders))
	for _, order := range orders {
		ids = append(ids, order.ID)
	}
	progress, err := s.orderRepo.ProgressByIDs(ids)
	if err != nil {
		return nil, 0, util.LogError(s.log, constants.LOG_GROUP_ORDER_DELIVERED, fmt.Errorf("load group order progress: %w", err))
	}
	items := make([]dto.GroupOrderListItem, 0, len(orders))
	for _, order := range orders {
		items = append(items, dto.GroupOrderListItem{GroupOrder: order, GroupOrderProgress: progress[order.ID]})
	}
	return items, total, nil
}

// GetOrder 查询团检订单详情和已归集体检人的报告状态。
func (s *EnterpriseService) GetOrder(ctx context.Context, orderID uint) (*dto.GroupOrderDetail, error) {
	order, err := s.getOrder(orderID)
	if err != nil {
		return nil, err
	}
	examinees, err := s.orderRepo.ExamineesByOrder(order)
	if err != nil {
		return nil, util.LogError(s.log, constants.LOG_GROUP_ORDER_DELIVERED, fmt.Errorf("load group order examinees: %w", err))
	}
	progressMap, err := s.orderRepo.ProgressByIDs([]uint{order.ID})
	if err != nil {
		return nil, util.LogError(s.log, constants.LOG_GROUP_ORDER_DELIVERED, fmt.Errorf("load group order progress: %w", err))
	}
	return &dto.GroupOrderDetail{
		GroupOrder:         *order,
		GroupOrderProgress: progressMap[order.ID],
		Examinees:          examinees,
	}, nil
}

// DeliverReports 报告批量交付。全部已发布后才允许交付；重复交付返回同一条完成结果。
func (s *EnterpriseService) DeliverReports(ctx context.Context, orderID uint) (*dto.GroupOrderDetail, error) {
	order, err := s.getOrder(orderID)
	if err != nil {
		return nil, err
	}

	// 幂等：已交付订单不重复更新交付时间，直接返回当前真实进度和完成结果。
	if order.ReportDeliveryStatus == constants.ReportDeliveryDelivered && order.DeliveredAt != nil {
		s.log.InfoContext(ctx, constants.LOG_GROUP_ORDER_DELIVERED, "order_id", orderID, "idempotent", true)
		result, err := s.GetOrder(ctx, orderID)
		if err != nil {
			return nil, err
		}
		result.DeliveryMessage = "该订单已完成交付"
		return result, nil
	}

	progressMap, err := s.orderRepo.ProgressByIDs([]uint{orderID})
	if err != nil {
		return nil, util.LogError(s.log, constants.LOG_GROUP_ORDER_DELIVERED, fmt.Errorf("load group order progress: %w", err))
	}
	progress := progressMap[orderID]
	if progress.RequiredCount == 0 {
		return nil, util.ConflictError("交付失败：该企业与套餐下还没有已登记体检人", errors.New("no registered examinees"))
	}
	if progress.PendingReportCount > 0 {
		return nil, util.ConflictError(fmt.Sprintf("交付失败：还差 %d 人的已发布报告（应交付 %d 人，已就绪 %d 人）", progress.PendingReportCount, progress.RequiredCount, progress.ReadyCount), errors.New("reports not ready"))
	}

	deliveredAt := time.Now()
	updated, err := s.orderRepo.MarkDelivered(orderID, deliveredAt)
	if err != nil {
		return nil, util.LogError(s.log, constants.LOG_GROUP_ORDER_DELIVERED, fmt.Errorf("deliver reports: %w", err))
	}
	if !updated {
		// 并发交付时复用先完成事务写入的订单结果。
		s.log.InfoContext(ctx, constants.LOG_GROUP_ORDER_DELIVERED, "order_id", orderID, "idempotent", true)
		result, err := s.GetOrder(ctx, orderID)
		if err != nil {
			return nil, err
		}
		result.DeliveryMessage = "该订单已完成交付"
		return result, nil
	}
	s.log.InfoContext(ctx, constants.LOG_GROUP_ORDER_DELIVERED, "order_id", orderID, "count", progress.RequiredCount, "delivered_at", deliveredAt)
	result, err := s.GetOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	result.DeliveryMessage = "全部报告已发布，订单交付完成"
	return result, nil
}

func (s *EnterpriseService) getOrder(orderID uint) (*model.GroupOrder, error) {
	order, err := s.orderRepo.FindByID(orderID)
	if err != nil {
		if errors.Is(err, util.ErrNotFound) {
			return nil, util.NotFoundError("团检订单（GroupOrder）不存在", err)
		}
		return nil, util.LogError(s.log, constants.LOG_GROUP_ORDER_CREATED, fmt.Errorf("find group order: %w", err))
	}
	return order, nil
}
