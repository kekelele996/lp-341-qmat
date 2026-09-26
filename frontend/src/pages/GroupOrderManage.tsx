import { useCallback, useEffect, useState } from 'react';
import { Button, Card, Descriptions, Drawer, Form, Input, InputNumber, Modal, Popconfirm, Progress, Select, Space, Table, Tag, message } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import { createEnterprise, createGroupOrder, deliverReports, getGroupOrder, listEnterprises, listGroupOrders } from '../api/enterprise';
import { listPackages } from '../api/package';
import type { Enterprise, GroupOrder, GroupOrderDetail, GroupOrderExaminee, Package } from '../types';
import StatusBadge from '../components/common/StatusBadge';
import ReportStatusBadge from '../components/common/ReportStatusBadge';
import EmptyState from '../components/common/EmptyState';
import { usePagination } from '../hooks/usePagination';
import { formatDateTime } from '../utils/dateFormat';

function ReportProgress({ order }: { order: GroupOrder }) {
  const percent = order.required_count > 0 ? Math.round((order.ready_count / order.required_count) * 100) : 0;
  return (
    <Space direction="vertical" size={2} style={{ minWidth: 170 }}>
      <Space size={4} wrap>
        <Tag>应交付 {order.required_count}</Tag>
        <Tag color="green">已就绪 {order.ready_count}</Tag>
        <Tag color={order.pending_report_count > 0 ? 'orange' : 'default'}>待出报告 {order.pending_report_count}</Tag>
      </Space>
      <Progress percent={percent} size="small" status={order.pending_report_count > 0 ? 'active' : 'success'} />
    </Space>
  );
}

export default function GroupOrderManage() {
  const [items, setItems] = useState<GroupOrder[]>([]);
  const [enterprises, setEnterprises] = useState<Enterprise[]>([]);
  const [packages, setPackages] = useState<Package[]>([]);
  const [loading, setLoading] = useState(false);
  const [detail, setDetail] = useState<GroupOrderDetail | null>(null);
  const [detailOpen, setDetailOpen] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const { pagination, setTotal, onPageChange } = usePagination(1, 10);
  const total = pagination.total;
  const [orderOpen, setOrderOpen] = useState(false);
  const [entOpen, setEntOpen] = useState(false);
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

  async function onView(order: GroupOrder) {
    setDetailOpen(true);
    setDetailLoading(true);
    try {
      setDetail(await getGroupOrder(order.id));
    } finally {
      setDetailLoading(false);
    }
  }

  async function onDeliver(order: GroupOrder) {
    const result = await deliverReports(order.id);
    message.success(result.delivery_message || `全部 ${result.required_count} 人的报告已发布并完成交付`);
    if (detail?.id === result.id) setDetail(result);
    load();
  }

  const columns: ColumnsType<GroupOrder> = [
    { title: '企业', render: (_, r) => r.enterprise?.name ?? '-' },
    { title: '套餐', render: (_, r) => r.package?.name ?? '-' },
    { title: '合同人数', dataIndex: 'examinee_count', width: 90 },
    { title: '报告进度', width: 230, render: (_, r) => <ReportProgress order={r} /> },
    { title: '订单状态', dataIndex: 'status', width: 100, render: (v) => <StatusBadge status={v} type="order" /> },
    { title: '交付状态', dataIndex: 'report_delivery_status', width: 100, render: (v) => <StatusBadge status={v} type="delivery" /> },
    { title: '交付时间', dataIndex: 'delivered_at', width: 160, render: (v) => formatDateTime(v) },
    {
      title: '操作',
      width: 160,
      render: (_, r) => (
        <Space>
          <a onClick={() => onView(r)}>详情</a>
          <Popconfirm
            title={r.report_delivery_status === 'delivered' ? '该订单已交付，是否再次查看交付结果？' : '确认交付全部报告？'}
            description={r.pending_report_count > 0 ? `还差 ${r.pending_report_count} 人报告发布，交付将失败。` : '只有全部报告发布后才能交付。'}
            onConfirm={() => onDeliver(r)}
          >
            <a>{r.report_delivery_status === 'delivered' ? '再次交付' : '批量交付报告'}</a>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  const examineeColumns: ColumnsType<GroupOrderExaminee> = [
    { title: '体检人', dataIndex: 'name' },
    { title: '手机号', dataIndex: 'phone', render: (v) => v || '-' },
    { title: '导检单号', dataIndex: 'guide_no', render: (v) => v || '-' },
    { title: '报告编号', dataIndex: 'report_no', render: (v) => v || '-' },
    { title: '报告状态', dataIndex: 'report_status', render: (v) => v ? <ReportStatusBadge status={v} /> : <Tag>待出报告</Tag> },
    { title: '就绪', dataIndex: 'ready', render: (v) => v ? <Tag color="green">已就绪</Tag> : <Tag color="orange">待出报告</Tag> },
  ];

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

      <Drawer
        title={detail ? `订单详情 #${detail.id}` : '订单详情'}
        width={860}
        open={detailOpen}
        onClose={() => setDetailOpen(false)}
        destroyOnClose
        extra={detail && (
          <Popconfirm
            title={detail.report_delivery_status === 'delivered' ? '该订单已交付，是否再次查看交付结果？' : '确认交付全部报告？'}
            description={detail.pending_report_count > 0 ? `还差 ${detail.pending_report_count} 人报告发布，交付将失败。` : undefined}
            onConfirm={() => onDeliver(detail)}
          >
            <Button type={detail.report_delivery_status === 'delivered' ? 'default' : 'primary'}>
              {detail.report_delivery_status === 'delivered' ? '再次交付' : '批量交付报告'}
            </Button>
          </Popconfirm>
        )}
      >
        {detail && (
          <Space direction="vertical" size={16} style={{ width: '100%' }}>
            <Descriptions bordered size="small" column={2}>
              <Descriptions.Item label="企业">{detail.enterprise?.name ?? '-'}</Descriptions.Item>
              <Descriptions.Item label="套餐">{detail.package?.name ?? '-'}</Descriptions.Item>
              <Descriptions.Item label="合同人数">{detail.examinee_count}</Descriptions.Item>
              <Descriptions.Item label="已登记/应交付">{detail.required_count}</Descriptions.Item>
              <Descriptions.Item label="已就绪">{detail.ready_count}</Descriptions.Item>
              <Descriptions.Item label="待出报告">{detail.pending_report_count}</Descriptions.Item>
              <Descriptions.Item label="订单状态"><StatusBadge status={detail.status} type="order" /></Descriptions.Item>
              <Descriptions.Item label="交付状态"><StatusBadge status={detail.report_delivery_status} type="delivery" /></Descriptions.Item>
              <Descriptions.Item label="创建时间">{formatDateTime(detail.created_at)}</Descriptions.Item>
              <Descriptions.Item label="交付时间">{formatDateTime(detail.delivered_at)}</Descriptions.Item>
            </Descriptions>
            <Card size="small" title="报告发布进度">
              <ReportProgress order={detail} />
            </Card>
            <Table
              rowKey="examinee_id"
              size="small"
              columns={examineeColumns}
              dataSource={detail.examinees}
              loading={detailLoading}
              pagination={false}
              locale={{ emptyText: <EmptyState description="该企业与套餐下暂无已登记体检人" /> }}
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
