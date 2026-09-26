package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/blueship581/gbcheckup/internal/constants"
	"github.com/blueship581/gbcheckup/internal/model"
	"github.com/blueship581/gbcheckup/internal/repository"
	"github.com/blueship581/gbcheckup/internal/util"
	"gorm.io/gorm"
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
	order := &model.GroupOrder{EnterpriseID: enterpriseID, PackageID: packageID, ExamineeCount: count, Status: constants.GroupOrderPending}
	if err := s.orderRepo.Create(order); err != nil {
		return nil, util.LogError(s.log, constants.LOG_GROUP_ORDER_CREATED, fmt.Errorf("create group order: %w", err))
	}
	s.log.InfoContext(ctx, constants.LOG_GROUP_ORDER_CREATED, "order_id", order.ID)
	return order, nil
}

// ListOrders 团检订单列表（挂载报告交付进度：应交付/已就绪/待出报告）。
func (s *EnterpriseService) ListOrders(ctx context.Context, page, pageSize int) ([]model.GroupOrder, int64, error) {
	orders, total, err := s.orderRepo.List(page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	summary, err := s.orderRepo.ReadinessSummary()
	if err != nil {
		return nil, 0, util.LogError(s.log, constants.LOG_GROUP_ORDER_DELIVER_FAILED, fmt.Errorf("list order readiness: %w", err))
	}
	for i := range orders {
		ready := summary[model.OrderKey{EnterpriseID: orders[i].EnterpriseID, PackageID: orders[i].PackageID}].ReadyCount
		attachDeliveryProgress(&orders[i], ready)
	}
	return orders, total, nil
}

// GetOrderDetail 团检订单详情：进度 + 按企业+套餐归集的已登记体检人报告状态。
func (s *EnterpriseService) GetOrderDetail(ctx context.Context, orderID uint) (*model.GroupOrder, []model.GroupOrderMember, error) {
	order, err := s.findOrder(orderID)
	if err != nil {
		return nil, nil, err
	}
	readiness, err := s.orderRepo.Readiness(order.EnterpriseID, order.PackageID)
	if err != nil {
		return nil, nil, util.LogError(s.log, constants.LOG_GROUP_ORDER_DELIVER_FAILED, fmt.Errorf("order readiness: %w", err))
	}
	members, err := s.orderRepo.ListMembers(order.EnterpriseID, order.PackageID)
	if err != nil {
		return nil, nil, util.LogError(s.log, constants.LOG_GROUP_ORDER_DELIVER_FAILED, fmt.Errorf("list order members: %w", err))
	}
	attachDeliveryProgress(order, readiness.ReadyCount)
	return order, members, nil
}

// DeliverReports 报告批量交付：全部报告发布后才允许交付；
// 订单状态、交付状态、交付时间在同一次更新中落库；重复交付返回同一条完成结果。
func (s *EnterpriseService) DeliverReports(ctx context.Context, orderID uint) (*model.GroupOrder, error) {
	order, err := s.findOrder(orderID)
	if err != nil {
		return nil, err
	}
	if order.ReportDeliveryStatus == constants.GroupOrderDeliveryDelivered {
		// 幂等：已交付订单重复交付直接返回当前完成状态。
		readiness, err := s.orderRepo.Readiness(order.EnterpriseID, order.PackageID)
		if err != nil {
			return nil, util.LogError(s.log, constants.LOG_GROUP_ORDER_DELIVER_FAILED, fmt.Errorf("order readiness: %w", err))
		}
		attachDeliveryProgress(order, readiness.ReadyCount)
		return order, nil
	}
	expected := int64(order.ExamineeCount)
	now := time.Now()
	err = s.orderRepo.Transaction(func(tx *gorm.DB) error {
		txRepo := s.orderRepo.WithTx(tx)
		readiness, err := txRepo.Readiness(order.EnterpriseID, order.PackageID)
		if err != nil {
			return fmt.Errorf("order readiness: %w", err)
		}
		if readiness.ReadyCount < expected {
			missing := expected - readiness.ReadyCount
			return util.NewAppError(constants.CodeGroupOrderNotReady, 409,
				fmt.Sprintf("%s：应交付 %d 人，已就绪 %d 人，还差 %d 人报告未发布", constants.MsgGroupOrderNotReady, expected, readiness.ReadyCount, missing),
				fmt.Errorf("GroupOrder[id=%d] report_delivery_status not ready: %d/%d published", orderID, readiness.ReadyCount, expected))
		}
		if _, err := txRepo.MarkDelivered(orderID, now); err != nil {
			return fmt.Errorf("mark order delivered: %w", err)
		}
		return nil
	})
	if err != nil {
		var appErr *util.AppError
		if errors.As(err, &appErr) {
			s.log.WarnContext(ctx, constants.LOG_GROUP_ORDER_DELIVER_FAILED, "order_id", orderID, "reason", appErr.Error())
			return nil, appErr
		}
		return nil, util.LogError(s.log, constants.LOG_GROUP_ORDER_DELIVER_FAILED, fmt.Errorf("deliver reports: %w", err))
	}
	// 重新读取，拿到同一次更新落库的订单状态、交付状态与交付时间。
	delivered, err := s.findOrder(orderID)
	if err != nil {
		return nil, err
	}
	readiness, err := s.orderRepo.Readiness(delivered.EnterpriseID, delivered.PackageID)
	if err != nil {
		return nil, util.LogError(s.log, constants.LOG_GROUP_ORDER_DELIVER_FAILED, fmt.Errorf("order readiness: %w", err))
	}
	attachDeliveryProgress(delivered, readiness.ReadyCount)
	s.log.InfoContext(ctx, constants.LOG_GROUP_ORDER_DELIVERED, "order_id", orderID, "ready_count", delivered.ReadyCount)
	return delivered, nil
}

// findOrder 查询订单并统一未找到错误。
func (s *EnterpriseService) findOrder(orderID uint) (*model.GroupOrder, error) {
	order, err := s.orderRepo.FindByID(orderID)
	if err != nil {
		if errors.Is(err, util.ErrNotFound) {
			return nil, util.NotFoundError(constants.MsgGroupOrderNotFound, err)
		}
		return nil, err
	}
	return order, nil
}

// attachDeliveryProgress 挂载交付进度：应交付=订单人数，待出报告=应交付-已就绪。
func attachDeliveryProgress(order *model.GroupOrder, readyCount int64) {
	order.ExpectedCount = order.ExamineeCount
	order.ReadyCount = int(readyCount)
	pending := order.ExamineeCount - order.ReadyCount
	if pending < 0 {
		pending = 0
	}
	order.PendingCount = pending
}
