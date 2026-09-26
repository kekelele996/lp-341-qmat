import request from '../utils/request';
import type { Enterprise, GroupOrder, GroupOrderDetail, PageData } from '../types';

export function listEnterprises(params: { page?: number; page_size?: number }): Promise<PageData<Enterprise>> {
  return request.get('/enterprises', { params });
}

export function createEnterprise(data: Partial<Enterprise>): Promise<Enterprise> {
  return request.post('/enterprises', data);
}

export function listGroupOrders(params: { page?: number; page_size?: number }): Promise<PageData<GroupOrder>> {
  return request.get('/enterprises/orders', { params });
}

export function getGroupOrder(id: number): Promise<GroupOrderDetail> {
  return request.get(`/enterprises/orders/${id}`);
}

export function createGroupOrder(data: { enterprise_id: number; package_id: number; examinee_count: number }): Promise<GroupOrder> {
  return request.post('/enterprises/orders', data);
}

export function deliverReports(id: number): Promise<GroupOrder> {
  return request.post(`/enterprises/orders/${id}/deliver`);
}
