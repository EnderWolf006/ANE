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
  Separator,
  Skeleton,
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
  title: ReactNode;
  description?: string;
  action?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <Card className={"panel " + className}>
      <Card.Header className="flex-row flex-wrap items-center justify-between gap-4">
        <div className="min-w-0">
          <Card.Title className="text-[15px] font-semibold tracking-tight">
            {title}
          </Card.Title>
          {description && (
            <Card.Description className="mt-1 text-xs leading-5 text-muted">
              {description}
            </Card.Description>
          )}
        </div>
        {action && <div className="shrink-0">{action}</div>}
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
      className="field w-full"
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
      {hint && <p className="mt-1 text-xs leading-5 text-muted">{hint}</p>}
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
    <div className="rounded-xl border border-border bg-surface-secondary/70 px-4 py-3 transition-colors hover:border-border/80">
      <Switch isSelected={selected} onChange={onChange} className="w-full">
        <Switch.Content className="w-full items-center justify-between gap-6">
          <div className="flex-1">
            <div className="text-[13px] font-medium text-foreground">
              {label}
            </div>
            {description && (
              <p className="mt-0.5 text-xs leading-5 text-muted">
                {description}
              </p>
            )}
          </div>
          <Switch.Control>
            <Switch.Thumb />
          </Switch.Control>
        </Switch.Content>
      </Switch>
    </div>
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
  const live = value === "online" || value === "retrying";
  return (
    <Chip size="sm" color={color} variant="soft" className="gap-1.5 font-medium">
      <span className={"status-dot " + (live ? "is-live" : "")} />
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
        <Alert.Description className="text-[13px] leading-6">
          {children}
        </Alert.Description>
      </Alert.Content>
    </Alert>
  );
}

export function Empty({
  title = "暂无记录",
  description = "新的记录会显示在这里。",
  action,
}: {
  title?: string;
  description?: string;
  action?: ReactNode;
}) {
  return (
    <div className="empty-state">
      <div className="empty-state-icon">
        <Inbox size={24} strokeWidth={1.6} />
      </div>
      <h3>{title}</h3>
      <p>{description}</p>
      {action && <div className="mt-2">{action}</div>}
    </div>
  );
}

export function DataTable({
  label,
  headers,
  rows,
  emptyTitle,
  emptyDescription,
}: {
  label: string;
  headers: string[];
  rows: ReactNode[][];
  emptyTitle?: string;
  emptyDescription?: string;
}) {
  if (!rows.length)
    return <Empty title={emptyTitle} description={emptyDescription} />;
  return (
    <div className="overflow-hidden rounded-2xl border border-border bg-surface">
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
    </div>
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
            <Modal.Heading className="text-base font-semibold tracking-tight">
              {title}
            </Modal.Heading>
          </Modal.Header>
          <Modal.Body className="space-y-4">{children}</Modal.Body>
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
    <div className="mt-5 flex items-center justify-between gap-4 border-t border-separator pt-4">
      <span className="text-xs text-muted tabular-nums">
        当前第 <strong className="font-semibold text-foreground">{page}</strong>{" "}
        页
      </span>
      <div className="flex items-center gap-2">
        <Button
          size="sm"
          variant="secondary"
          isDisabled={page <= 1}
          onPress={() => onPage(page - 1)}
        >
          <ChevronLeft size={15} />
          上一页
        </Button>
        <Button
          size="sm"
          variant="secondary"
          isDisabled={!more}
          onPress={() => onPage(page + 1)}
        >
          下一页
          <ChevronRight size={15} />
        </Button>
      </div>
    </div>
  );
}

export function Loading({ label = "正在读取数据" }: { label?: string }) {
  return (
    <div className="flex min-h-48 flex-col items-center justify-center gap-3 text-sm text-muted">
      <Spinner size="md" color="accent" />
      <span className="text-xs tracking-wide">{label}</span>
    </div>
  );
}

export function SkeletonMetrics() {
  return (
    <div className="space-y-5">
      <div className="metrics-grid">
        {Array.from({ length: 4 }).map((_, i) => (
          <Card key={i} className="metric-card">
            <Card.Content className="space-y-3">
              <div className="flex items-center justify-between">
                <Skeleton className="h-3.5 w-16 rounded-md" />
                <Skeleton className="h-8 w-8 rounded-lg" />
              </div>
              <Skeleton className="h-8 w-28 rounded-lg" />
              <Skeleton className="h-3 w-24 rounded-md" />
            </Card.Content>
          </Card>
        ))}
      </div>
      <div className="overview-columns">
        <Card className="panel">
          <Card.Content className="space-y-4">
            <Skeleton className="h-5 w-32 rounded-md" />
            <Skeleton className="h-16 w-full rounded-xl" />
            <Skeleton className="h-12 w-full rounded-xl" />
          </Card.Content>
        </Card>
        <Card className="panel">
          <Card.Content className="space-y-3">
            <Skeleton className="h-5 w-28 rounded-md" />
            <Skeleton className="h-14 w-full rounded-xl" />
            <Skeleton className="h-14 w-full rounded-xl" />
          </Card.Content>
        </Card>
      </div>
    </div>
  );
}

export function SkeletonTable({ rows = 5 }: { rows?: number }) {
  return (
    <div className="space-y-3 py-2">
      <Skeleton className="h-10 w-full rounded-xl" />
      {Array.from({ length: rows }).map((_, i) => (
        <Skeleton key={i} className="h-12 w-full rounded-xl" />
      ))}
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

export function KV({
  label,
  value,
  mono = false,
}: {
  label: string;
  value: ReactNode;
  mono?: boolean;
}) {
  return (
    <div className="space-y-1">
      <dt className="text-[11px] font-medium text-muted">{label}</dt>
      <dd
        className={
          "break-words text-[13px] font-medium text-foreground " +
          (mono ? "font-mono text-xs" : "")
        }
      >
        {value}
      </dd>
    </div>
  );
}

export { Separator };
