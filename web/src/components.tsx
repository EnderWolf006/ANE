import { useEffect, useState, type ReactNode } from "react";
import {
  Alert,
  Button,
  Card,
  Chip,
  Input,
  Label,
  ListBox,
  Modal,
  Select,
  Spinner,
  Switch,
  Table,
  TextField,
} from "@heroui/react";
import { ChevronLeft, ChevronRight, Inbox } from "lucide-react";
export function Panel({
  title,
  description,
  action,
  children,
  className = "",
}: {
  title: string;
  description?: string;
  action?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <Card className={"panel " + className}>
      <Card.Header className="flex-row items-center justify-between gap-4">
        <div>
          <Card.Title className="text-base">{title}</Card.Title>
          {description && (
            <Card.Description className="mt-1 text-xs leading-6">
              {description}
            </Card.Description>
          )}
        </div>
        {action}
      </Card.Header>
      <Card.Content>{children}</Card.Content>
    </Card>
  );
}
export function Field({
  label,
  value,
  onChange,
  type = "text",
  hint,
  required = false,
  readOnly = false,
  min,
  max,
  placeholder,
}: {
  label: string;
  value: string;
  onChange?: (v: string) => void;
  type?: "text" | "password" | "email" | "number" | "url";
  hint?: string;
  required?: boolean;
  readOnly?: boolean;
  min?: number;
  max?: number;
  placeholder?: string;
}) {
  return (
    <TextField
      className="w-full"
      value={value}
      onChange={onChange}
      type={type}
      isRequired={required}
      isReadOnly={readOnly}
    >
      <Label>{label}</Label>
      <Input
        min={min}
        max={max}
        placeholder={placeholder}
        autoComplete={type === "password" ? "new-password" : undefined}
      />
      {hint && <p className="text-xs leading-5 text-muted">{hint}</p>}
    </TextField>
  );
}
export function Choice({
  label,
  value,
  onChange,
  options,
  className = "",
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: { id: string; label: string }[];
  className?: string;
}) {
  return (
    <Select
      aria-label={label}
      value={value}
      onChange={(v) => onChange(String(v))}
      className={className}
    >
      <Label>{label}</Label>
      <Select.Trigger>
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover>
        <ListBox>
          {options.map((o) => (
            <ListBox.Item id={o.id} key={o.id} textValue={o.label}>
              {o.label}
              <ListBox.ItemIndicator />
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  );
}
export function Toggle({
  label,
  description,
  selected,
  onChange,
}: {
  label: string;
  description?: string;
  selected: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <Switch isSelected={selected} onChange={onChange} className="w-full">
      <Switch.Content className="w-full justify-between gap-6">
        <div className="flex-1">
          <div className="text-sm font-medium">{label}</div>
          {description && (
            <p className="mt-1 text-xs leading-5 text-muted">{description}</p>
          )}
        </div>
        <Switch.Control>
          <Switch.Thumb />
        </Switch.Control>
      </Switch.Content>
    </Switch>
  );
}
const names: Record<string, string> = {
  pending: "待处理",
  paid: "已付款",
  expired: "已超时",
  matched: "已匹配",
  unmatched: "未匹配",
  ignored: "已忽略",
  success: "已送达",
  retrying: "重试中",
  failed: "失败",
  waiting: "等待付款",
  online: "在线",
  offline: "离线",
  revoked: "已撤销",
  sent: "已发送",
  cancelled: "已取消",
  enabled: "已启用",
  disabled: "已停用",
};
export function Status({ value }: { value: string }) {
  const color = [
    "paid",
    "matched",
    "success",
    "online",
    "sent",
    "enabled",
  ].includes(value)
    ? "success"
    : ["failed", "revoked", "disabled"].includes(value)
      ? "danger"
      : ["pending", "retrying", "unmatched", "offline"].includes(value)
        ? "warning"
        : "default";
  return (
    <Chip size="sm" color={color} variant="soft">
      {names[value] || value}
    </Chip>
  );
}
export function Notice({
  children,
  error = false,
}: {
  children: ReactNode;
  error?: boolean;
}) {
  return (
    <Alert className="ane-notice" status={error ? "danger" : "accent"}>
      <Alert.Indicator />
      <Alert.Content>
        <Alert.Description>{children}</Alert.Description>
      </Alert.Content>
    </Alert>
  );
}
export function Empty({
  title = "暂无记录",
  description = "新的记录会显示在这里。",
}: {
  title?: string;
  description?: string;
}) {
  return (
    <div className="empty-state">
      <Inbox size={30} strokeWidth={1.4} />
      <h3>{title}</h3>
      <p>{description}</p>
    </div>
  );
}
export function DataTable({
  label,
  headers,
  rows,
}: {
  label: string;
  headers: string[];
  rows: ReactNode[][];
}) {
  if (!rows.length) return <Empty />;
  return (
    <Table>
      <Table.ScrollContainer>
        <Table.Content aria-label={label} className="min-w-[640px]">
          <Table.Header>
            {headers.map((h, i) => (
              <Table.Column key={h} isRowHeader={i === 0}>
                {h}
              </Table.Column>
            ))}
          </Table.Header>
          <Table.Body>
            {rows.map((row, i) => (
              <Table.Row key={i}>
                {row.map((cell, j) => (
                  <Table.Cell key={j}>{cell}</Table.Cell>
                ))}
              </Table.Row>
            ))}
          </Table.Body>
        </Table.Content>
      </Table.ScrollContainer>
    </Table>
  );
}
export function Dialog({
  title,
  open,
  onClose,
  children,
  footer,
  className = "sm:max-w-xl",
}: {
  title: string;
  open: boolean;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
  className?: string;
}) {
  return (
    <Modal.Backdrop
      isOpen={open}
      onOpenChange={(v) => {
        if (!v) onClose();
      }}
    >
      <Modal.Container>
        <Modal.Dialog className={className}>
          <Modal.CloseTrigger />
          <Modal.Header>
            <Modal.Heading>{title}</Modal.Heading>
          </Modal.Header>
          <Modal.Body className="space-y-5">{children}</Modal.Body>
          <Modal.Footer>
            {footer || (
              <Button variant="secondary" onPress={onClose}>
                关闭
              </Button>
            )}
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}
export function Pagination({
  page,
  more,
  onPage,
}: {
  page: number;
  more: boolean;
  onPage: (n: number) => void;
}) {
  return (
    <div className="mt-5 flex items-center justify-end gap-4">
      <Button
        size="sm"
        variant="tertiary"
        isDisabled={page <= 1}
        onPress={() => onPage(page - 1)}
      >
        <ChevronLeft size={15} />
        上一页
      </Button>
      <span className="text-xs text-muted">第 {page} 页</span>
      <Button
        size="sm"
        variant="tertiary"
        isDisabled={!more}
        onPress={() => onPage(page + 1)}
      >
        下一页
        <ChevronRight size={15} />
      </Button>
    </div>
  );
}
export function Loading() {
  return (
    <div className="flex min-h-48 items-center justify-center gap-3 text-sm text-muted">
      <Spinner size="sm" />
      正在读取数据
    </div>
  );
}
export function useLoad<T>(fetcher: () => Promise<T>, deps: unknown[]) {
  const [data, setData] = useState<T>();
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    let active = true;
    setLoading(true);
    setError("");
    fetcher()
      .then((v) => {
        if (active) setData(v);
      })
      .catch((e) => {
        if (active) setError(String(e.message));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, deps);
  return { data, error, loading, setData };
}
export function KV({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="space-y-1.5">
      <dt className="text-xs text-muted">{label}</dt>
      <dd className="break-words text-sm">{value}</dd>
    </div>
  );
}
