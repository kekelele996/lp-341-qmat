import { useCallback, useEffect, useState } from 'react';
import { Button, Card, Descriptions, Drawer, Form, Input, InputNumber, Modal, Progress, Select, Space, Table, Tag, message } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import { createEnterprise, createGroupOrder, deliverReports, getGroupOrder, listEnterprises, listGroupOrders } from '../api/enterprise';
import { listPackages } from '../api/package';
import type { Enterprise, GroupOrder, GroupOrderDetail, GroupOrderMember, Package } from '../types';
import StatusBadge from '../components/common/StatusBadge';
import ReportStatusBadge from '../components/common/ReportStatusBadge';
import EmptyState from '../components/common/EmptyState';
import { usePagination } from '../hooks/usePagination';
import { formatDateTime } from '../utils/dateFormat';

// 交付进度条：已就绪 / 应交付
function DeliveryProgress({ order }: { order: GroupOrder }) {
  const expected = order.expected_count ?? order.examinee_count;
  const ready = order.ready_count ?? 0;
  const percent = expected > 0 ? Math.round((ready / expected) * 100) : 0;
  return <Progress percent={Math.min(percent, 100)} size="small" status={order.report_delivery_status === 'delivered' ? 'success' : 'active'} />;
}

export default function GroupOrderManage() {
  const [items, setItems] = useState<GroupOrder[]>([]);
  const [enterprises, setEnterprises] = useState<Enterprise[]>([]);
  const [packages, setPackages] = useState<Package[]>([]);
  const [loading, setLoading] = useState(false);
  const { pagination, setTotal, onPageChange } = usePagination(1, 10);
  const total = pagination.total;
  const [orderOpen, setOrderOpen] = useState(false);
  const [entOpen, setEntOpen] = useState(false);
  const [detail, setDetail] = useState<GroupOrderDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [orderForm] = Form.useForm();
  const [entForm] = Form.useForm();

  const load = useCallback(async (page = pagination.page, size = pagination.pageSize) => {
    setLoading(true);
    try {
      const data = await listGroupOrders({ page, page_size: size });
      setItems(data.list);
      setTotal(data.total);
      const ent = await listEnterprises({ page_size: 100 });
      setEnterprises(ent.list);
      const p = await listPackages({ page_size: 100 });
      setPackages(p.list);
    } finally {
      setLoading(false);
    }
  }, [pagination.page, pagination.pageSize, setTotal]);

  useEffect(() => { load(); }, [load]);

  async function onCreateOrder(values: { enterprise_id: number; package_id: number; examinee_count: number }) {
    await createGroupOrder(values);
    message.success('团检订单已创建');
    setOrderOpen(false);
    orderForm.resetFields();
    load();
  }

  async function onCreateEnterprise(values: Partial<Enterprise>) {
    await createEnterprise(values);
    message.success('企业已创建');
    setEntOpen(false);
    entForm.resetFields();
    load();
  }

  async function openDetail(order: GroupOrder) {
    setDetailLoading(true);
    try {
      setDetail(await getGroupOrder(order.id));
    } finally {
      setDetailLoading(false);
    }
  }

  function onDeliver(order: GroupOrder) {
    const expected = order.expected_count ?? order.examinee_count;
    const ready = order.ready_count ?? 0;
    Modal.confirm({
      title: '批量交付报告',
      content: `应交付 ${expected} 人，已就绪 ${ready} 人，待出报告 ${Math.max(expected - ready, 0)} 人。全部报告发布后才能交付，确认交付？`,
      onOk: async () => {
        // 报告未发齐时后端返回 409 并说明还差几人，由请求拦截器统一提示
        await deliverReports(order.id);
        message.success('报告已批量交付');
        load();
      },
    });
  }

  const columns: ColumnsType<GroupOrder> = [
    { title: '企业', render: (_, r) => r.enterprise?.name ?? '-' },
    { title: '套餐', render: (_, r) => r.package?.name ?? '-' },
    { title: '应交付', render: (_, r) => r.expected_count ?? r.examinee_count },
    { title: '已就绪', render: (_, r) => r.ready_count ?? 0 },
    { title: '待出报告', render: (_, r) => r.pending_count ?? 0 },
    { title: '交付进度', width: 140, render: (_, r) => <DeliveryProgress order={r} /> },
    { title: '订单状态', dataIndex: 'status', render: (v) => <StatusBadge status={v} type="order" /> },
    { title: '交付状态', dataIndex: 'report_delivery_status', render: (v) => <StatusBadge status={v} type="delivery" /> },
    { title: '交付时间', dataIndex: 'delivered_at', render: (v) => formatDateTime(v) },
    { title: '操作', render: (_, r) => (
      <Space>
        <a onClick={() => openDetail(r)}>详情</a>
        {r.report_delivery_status !== 'delivered' && <a onClick={() => onDeliver(r)}>批量交付报告</a>}
      </Space>
    ) },
  ];

  const memberColumns: ColumnsType<GroupOrderMember> = [
    { title: '体检人', dataIndex: 'examinee_name' },
    { title: '身份证号', dataIndex: 'id_card_no' },
    { title: '导检单号', dataIndex: 'guide_no' },
    { title: '登记状态', dataIndex: 'registration_status', render: (v) => <StatusBadge status={v} type="registration" /> },
    { title: '报告状态', dataIndex: 'report_status', render: (v) => (v ? <ReportStatusBadge status={v} /> : <Tag>未生成</Tag>) },
  ];

  const detailOrder = detail?.order;

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Card size="small">
        <Space>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setOrderOpen(true)}>创建团检订单</Button>
          <Button onClick={() => setEntOpen(true)}>新增企业</Button>
        </Space>
      </Card>
      <Card size="small" title="团检订单列表">
        <Table rowKey="id" columns={columns} dataSource={items} loading={loading} pagination={{ current: pagination.page, pageSize: pagination.pageSize, total, onChange: onPageChange }} locale={{ emptyText: <EmptyState /> }} />
      </Card>

      <Drawer open={!!detail} width={720} title="团检订单详情" onClose={() => setDetail(null)} loading={detailLoading}>
        {detailOrder && (
          <Space direction="vertical" size={16} style={{ width: '100%' }}>
            <Descriptions column={2} size="small" bordered>
              <Descriptions.Item label="企业">{detailOrder.enterprise?.name ?? '-'}</Descriptions.Item>
              <Descriptions.Item label="套餐">{detailOrder.package?.name ?? '-'}</Descriptions.Item>
              <Descriptions.Item label="订单状态"><StatusBadge status={detailOrder.status} type="order" /></Descriptions.Item>
              <Descriptions.Item label="交付状态"><StatusBadge status={detailOrder.report_delivery_status} type="delivery" /></Descriptions.Item>
              <Descriptions.Item label="应交付">{detailOrder.expected_count ?? detailOrder.examinee_count} 人</Descriptions.Item>
              <Descriptions.Item label="已就绪">{detailOrder.ready_count ?? 0} 人</Descriptions.Item>
              <Descriptions.Item label="待出报告">{detailOrder.pending_count ?? 0} 人</Descriptions.Item>
              <Descriptions.Item label="交付时间">{formatDateTime(detailOrder.delivered_at)}</Descriptions.Item>
            </Descriptions>
            <DeliveryProgress order={detailOrder} />
            <Table
              rowKey="registration_id"
              size="small"
              title={() => '已登记体检人（按企业 + 套餐归集）'}
              columns={memberColumns}
              dataSource={detail?.members ?? []}
              pagination={false}
              locale={{ emptyText: <EmptyState description="该企业暂无该套餐的登记记录" /> }}
            />
          </Space>
        )}
      </Drawer>

      <Modal open={orderOpen} title="创建团检订单" onOk={() => orderForm.submit()} onCancel={() => setOrderOpen(false)} destroyOnClose>
        <Form form={orderForm} layout="vertical" onFinish={onCreateOrder}>
          <Form.Item name="enterprise_id" label="企业" rules={[{ required: true }]}>
            <Select options={enterprises.map((e) => ({ value: e.id, label: e.name }))} />
          </Form.Item>
          <Form.Item name="package_id" label="套餐" rules={[{ required: true }]}>
            <Select options={packages.map((p) => ({ value: p.id, label: p.name }))} />
          </Form.Item>
          <Form.Item name="examinee_count" label="人数" rules={[{ required: true }]}><InputNumber min={1} style={{ width: '100%' }} /></Form.Item>
        </Form>
      </Modal>

      <Modal open={entOpen} title="新增团检企业" onOk={() => entForm.submit()} onCancel={() => setEntOpen(false)} destroyOnClose>
        <Form form={entForm} layout="vertical" onFinish={onCreateEnterprise}>
          <Form.Item name="name" label="企业名称" rules={[{ required: true }]}><Input /></Form.Item>
          <Form.Item name="contact" label="联系人"><Input /></Form.Item>
          <Form.Item name="phone" label="联系电话"><Input /></Form.Item>
          <Form.Item name="address" label="地址"><Input /></Form.Item>
        </Form>
      </Modal>
    </Space>
  );
}
