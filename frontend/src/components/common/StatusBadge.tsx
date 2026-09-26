import { Tag } from 'antd';
import { DeliveryStatusLabels, GroupOrderStatusLabels, RegistrationStatusLabels } from '../../constants/report';

interface Props {
  status?: string;
  type?: 'registration' | 'package' | 'order' | 'delivery';
}

const map: Record<string, Record<string, string>> = {
  registration: { registered: 'blue', in_progress: 'orange', completed: 'green' },
  package: { active: 'green', inactive: 'default' },
  order: { pending: 'orange', confirmed: 'blue', done: 'green' },
  delivery: { pending: 'orange', delivered: 'green' },
};

const labels: Record<string, Record<string, string>> = {
  registration: RegistrationStatusLabels,
  order: GroupOrderStatusLabels,
  delivery: DeliveryStatusLabels,
};

// 看板/套餐/登记/团检 共用状态徽标
export default function StatusBadge({ status, type = 'registration' }: Props) {
  const m = map[type] ?? {};
  const color = m[status ?? ''] ?? 'default';
  const label = labels[type]?.[status ?? ''] ?? status ?? '-';
  return <Tag color={color}>{label}</Tag>;
}
