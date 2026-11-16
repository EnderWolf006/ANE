import { useEffect, useState, type ReactNode } from "react";
import { Button, Card, Chip, TextArea, TextField, Label } from "@heroui/react";
import {
  Activity,
  ArrowDownRight,
  ArrowRight,
  ArrowUpRight,
  Battery,
  BatteryCharging,
  Bell,
  Check,
  CheckCircle2,
  Clock3,
  Copy,
  KeyRound,
  Link2,
  Mail,
  Plus,
  Power,
  Radio,
  RotateCw,
  Search,
  ShieldCheck,
  Smartphone,
  Upload,
  Wifi,
} from "lucide-react";
import {
  api,
  maskAppKey,
  money,
  scoped,
  time,
  type Merchant,
  type Order,
  type Device,
  type EventRecord,
  type Pool,
  type SMTP,
} from "./api";
import {
  Choice,
  DataTable,
  Dialog,
  Empty,
  Field,
  KV,
  Loading,
  Notice,
  Pagination,
  Panel,
  Status,
  Toggle,
  useLoad,
} from "./components";
import { TimeRange } from "./DateTime";
import type { PageProps } from "./App";

type Dashboard = {
  orders: number;
  paid: number;
  pending: number;
  failed_callbacks: number;
  revenue_cents: number;
  discount_cents: number;
  today_cents: number;
  unmatched: number;
  online_devices: number;
};
function Guard({
  loading,
  error,
  children,
}: {
  loading: boolean;
  error: string;
  children: ReactNode;
}) {
  return error ? (
    <Notice error>{error}</Notice>
  ) : loading ? (
    <Loading />
  ) : (
    <>{children}</>
  );
}
function Metric({
  label,
  value,
  note,
  icon,
}: {
  label: string;
  value: string;
  note: string;
  icon: ReactNode;
}) {
  return (
    <Card className="metric-card">
      <Card.Content>
        <div className="flex items-center justify-between text-xs text-muted">
          {label}
          <span className="metric-icon">{icon}</span>
        </div>
        <div className="metric-value">{value}</div>
        <p className="text-[11px] text-muted">{note}</p>
      </Card.Content>
    </Card>
  );
}
function OrderTable({
  label,
  orders,
  onSelect,
}: {
  label: string;
  orders: Order[];
  onSelect: (order: Order) => void;
}) {
  return (
    <DataTable
      label={label}
      headers={[
        "订单",
        "应用 / 支付宝用户名",
        "原金额",
        "实收",
        "状态",
        "回调",
        "时间",
      ]}
      rows={orders.map((o) => [
        <Button
          size="sm"
          variant="ghost"
          className="h-auto justify-start p-0 font-mono text-xs"
          onPress={() => onSelect(o)}
        >
          {o.out_trade_no}
        </Button>,
        <div>
          {o.app_id || "默认应用"}
          <p className="text-xs text-muted">{o.payer || "—"}</p>
        </div>,
        money(o.amount_cents),
        <div>
          {money(o.pay_amount_cents)}
          {o.discount_cents > 0 && (
            <p className="text-xs text-success">
              优惠 {money(o.discount_cents)}
            </p>
          )}
        </div>,
        <Status value={o.status} />,
        <Status value={o.callback_status} />,
        <span className="text-xs text-muted">{time(o.created_at)}</span>,
      ])}
    />
  );
}
export function Overview(p: PageProps) {
  const [selected, setSelected] = useState<Order>();
  const r = useLoad(
    () =>
      Promise.all([
        api<{ data: Dashboard }>(scoped("dashboard", p.merchant)),
        api<{ data: Order[] }>(scoped("orders", p.merchant)),
        api<{ data: Device[] }>(scoped("device", p.merchant)),
      ]),
    [p.merchant, p.refresh],
  );
  const d = r.data?.[0].data,
    device = r.data?.[2].data[0];
  return (
    <Guard {...r}>
      {d && (
        <>
          <div className="metrics-grid">
            <Metric
              label="累计实收"
              value={money(d.revenue_cents)}
              note={"今日实收 " + money(d.today_cents)}
              icon={<ArrowUpRight size={17} />}
            />
            <Metric
              label="已付订单"
              value={String(d.paid)}
              note={"共 " + d.orders + " 笔交易订单"}
              icon={<CheckCircle2 size={17} />}
            />
            <Metric
              label="待付款订单"
              value={String(d.pending)}
              note="有效期内占用优惠金额位"
              icon={<Clock3 size={17} />}
            />
            <Metric
              label="累计优惠"
              value={money(d.discount_cents)}
              note="仅统计已付款订单的让利"
              icon={<ArrowDownRight size={17} />}
            />
          </div>
          <div className="overview-columns">
            <Panel
              title="监听设备"
              description="心跳与收款端健康状态"
              action={<Status value={device?.online ? "online" : "offline"} />}
            >
              {device ? (
                <>
                  <div className="device-summary">
                    <div className="phone-mark">
                      <Smartphone size={30} strokeWidth={1.3} />
                    </div>
                    <div>
                      <h3 className="font-semibold">{device.name}</h3>
                      <p className="mt-1 text-xs text-muted">
                        {device.model || "等待首次设备连接"}
                      </p>
                    </div>
                  </div>
                  <div className="grid grid-cols-3 gap-4 border-t border-border pt-5">
                    <KV
                      label="电量"
                      value={device.battery < 0 ? "—" : device.battery + "%"}
                    />
                    <KV
                      label="网络"
                      value={networkLabel(device.network, device.wifi_ssid)}
                    />
                    <KV
                      label="监听"
                      value={device.listener_connected ? "已连接" : "未连接"}
                    />
                  </div>
                  <p className="mt-5 text-[11px] text-muted">
                    最近心跳 {time(device.last_seen)}
                  </p>
                </>
              ) : (
                <Empty
                  title="连接你的收款设备"
                  description="在监听设备页面生成令牌，然后配置 ANE。"
                />
              )}
            </Panel>
            <Panel title="需要关注" description="及时处理异常，保持收款顺畅">
              <div className="attention-row">
                <div className="attention-icon">
                  <Activity size={18} />
                </div>
                <div className="flex-1">
                  <h3>未匹配收款</h3>
                  <p>核对金额与支付宝账单</p>
                </div>
                <b>{d.unmatched}</b>
              </div>
              <div className="attention-row">
                <div className="attention-icon">
                  <Link2 size={18} />
                </div>
                <div className="flex-1">
                  <h3>回调失败</h3>
                  <p>检查业务接口后可以重试</p>
                </div>
                <b>{d.failed_callbacks}</b>
              </div>
              <div className="rounded-xl bg-surface-secondary p-4 text-xs leading-6 text-muted">
                <Bell size={14} className="mr-2 inline" />
                前往通知订阅，开启设备离线和回调失败提醒。
              </div>
            </Panel>
          </div>
          <Panel
            title="最近交易"
            description="最近 5 笔订单及其支付状态"
            action={
              <Button
                size="sm"
                variant="ghost"
                onPress={() => p.navigate("orders")}
              >
                查看更多
                <ArrowRight size={14} />
              </Button>
            }
          >
            <OrderTable
              label="最近交易"
              orders={(r.data?.[1].data || []).slice(0, 5)}
              onSelect={setSelected}
            />
            <OrderDetails
              p={p}
              order={selected}
              onClose={() => setSelected(undefined)}
            />
          </Panel>
        </>
      )}
    </Guard>
  );
}
const initialOrderFilters = {
  status: "all",
  callback_status: "all",
  app: "all",
  discount: "all",
  start: "",
  end: "",
  sort: "newest",
};
export function Orders(p: PageProps) {
  const [page, setPage] = useState(1),
    [query, setQuery] = useState(""),
    [search, setSearch] = useState(""),
    [filters, setFilters] = useState(initialOrderFilters),
    [selected, setSelected] = useState<Order>();
  const apps = useLoad(
    () =>
      api<{ data: { name: string }[] }>(scoped("orders/filters", p.merchant)),
    [p.merchant, p.refresh],
  );
  const paramsForOrders = () => {
    const params = new URLSearchParams({
      page: String(page),
      q: search,
      sort: filters.sort,
    });
    for (const key of ["status", "callback_status", "discount"] as const)
      if (filters[key] !== "all") params.set(key, filters[key]);
    if (filters.app !== "all") params.set("app_id", filters.app.slice(4));
    if (filters.start) params.set("start", filters.start);
    if (filters.end) params.set("end", filters.end);
    return params;
  };
  const r = useLoad(
    () =>
      api<{ data: Order[]; has_more: boolean }>(
        scoped("orders?" + paramsForOrders(), p.merchant),
      ),
    [p.refresh, p.merchant, page, search, filters],
  );
  const change = (key: keyof typeof filters, value: string) => {
    setFilters((f) => ({ ...f, [key]: value }));
    setPage(1);
  };
  return (
    <>
      <Panel
        title="交易记录"
        description="点击订单号查看完整信息与回调记录"
        action={
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="tertiary"
              onPress={() => {
                setFilters(initialOrderFilters);
                setQuery("");
                setSearch("");
                setPage(1);
              }}
            >
              重置筛选
            </Button>
            <Button
              size="sm"
              variant="secondary"
              isPending={p.busy}
              onPress={() =>
                p.run(async () => {
                  const params = paramsForOrders();
                  params.set("export", "csv");
                  const response = await fetch(
                    "/api/manage/" + scoped("orders?" + params, p.merchant),
                    { credentials: "same-origin" },
                  );
                  if (!response.ok)
                    throw new Error((await response.json()).msg || "导出失败");
                  const url = URL.createObjectURL(await response.blob());
                  const a = document.createElement("a");
                  a.href = url;
                  a.download = "交易记录.csv";
                  a.click();
                  setTimeout(() => URL.revokeObjectURL(url), 1000);
                }, "导出完成")
              }
            >
              导出 CSV
            </Button>
          </div>
        }
      >
        <div className="mb-5 overflow-x-auto pb-1">
          <div className="order-filters">
            <form
              className="filter-search flex items-end gap-2"
              onSubmit={(e) => {
                e.preventDefault();
                setPage(1);
                setSearch(query);
              }}
            >
              <Field
                label="搜索订单"
                value={query}
                onChange={setQuery}
                placeholder="订单号、商品、应用或用户名"
              />
              <Button
                type="submit"
                isIconOnly
                variant="secondary"
                aria-label="搜索订单"
              >
                <Search size={17} />
              </Button>
            </form>
            <Choice
              label="支付状态"
              value={filters.status}
              onChange={(v) => change("status", v)}
              options={[
                { id: "all", label: "全部状态" },
                { id: "pending", label: "待付款" },
                { id: "paid", label: "已付款" },
                { id: "expired", label: "已过期" },
              ]}
            />
            <Choice
              label="回调状态"
              value={filters.callback_status}
              onChange={(v) => change("callback_status", v)}
              options={[
                { id: "all", label: "全部回调" },
                { id: "waiting", label: "等待付款" },
                { id: "pending", label: "待回调" },
                { id: "retrying", label: "重试中" },
                { id: "success", label: "成功" },
                { id: "failed", label: "失败" },
              ]}
            />
            <Choice
              label="应用"
              value={filters.app}
              onChange={(v) => change("app", v)}
              options={[
                { id: "all", label: "全部应用" },
                ...(apps.data?.data || []).map((app) => ({
                  id: "app:" + app.name,
                  label: app.name,
                })),
              ]}
            />
            <Choice
              label="优惠情况"
              value={filters.discount}
              onChange={(v) => change("discount", v)}
              options={[
                { id: "all", label: "全部订单" },
                { id: "none", label: "无优惠" },
                { id: "discounted", label: "有优惠" },
              ]}
            />
            <div className="filter-range">
              <TimeRange
                label="创建时间"
                start={filters.start}
                end={filters.end}
                onChange={(start, end) => {
                  setFilters((f) => ({ ...f, start, end }));
                  setPage(1);
                }}
              />
            </div>
            <Choice
              label="排序"
              value={filters.sort}
              onChange={(v) => change("sort", v)}
              options={[
                { id: "newest", label: "最新优先" },
                { id: "oldest", label: "最早优先" },
                { id: "amount_desc", label: "金额从高到低" },
                { id: "amount_asc", label: "金额从低到高" },
              ]}
            />
          </div>
        </div>

        {apps.error && <Notice error>{apps.error}</Notice>}
        <Guard {...r}>
          <OrderTable
            label="交易订单"
            orders={r.data?.data || []}
            onSelect={setSelected}
          />
          <Pagination
            page={page}
            more={r.data?.has_more || false}
            onPage={setPage}
          />
        </Guard>
      </Panel>
      <OrderDetails
        p={p}
        order={selected}
        onClose={() => setSelected(undefined)}
      />
    </>
  );
}
function OrderDetails({
  p,
  order: selected,
  onClose,
}: {
  p: PageProps;
  order?: Order;
  onClose: () => void;
}) {
  return (
    <Dialog title="订单详情" open={!!selected} onClose={onClose}>
      {selected && (
        <>
          <div className="grid grid-cols-2 gap-5">
            <KV label="商户订单号" value={selected.out_trade_no} />
            <KV label="应用" value={selected.app_id || "默认应用"} />
            <KV label="支付宝用户名" value={selected.payer || "—"} />
            <KV label="业务用户名" value={selected.username || "—"} />
            <KV label="商品" value={selected.name} />
            <KV label="原金额" value={money(selected.amount_cents)} />
            <KV label="实收" value={money(selected.pay_amount_cents)} />
            {selected.discount_cents > 0 && (
              <KV label="优惠" value={money(selected.discount_cents)} />
            )}
            <KV label="订单状态" value={<Status value={selected.status} />} />
            <KV
              label="回调状态"
              value={<Status value={selected.callback_status} />}
            />
            <KV label="创建时间" value={time(selected.created_at)} />
            <KV label="付款时间" value={time(selected.paid_at)} />
          </div>
          <KV label="平台订单号" value={selected.trade_no} />
          <KV label="异步回调" value={selected.notify_url} />
          <KV label="同步返回" value={selected.return_url || "—"} />
          <KV label="附加参数" value={selected.param || "—"} />
          <div className="rounded-xl bg-surface-secondary p-4">
            <p className="mb-2 text-xs text-muted">
              回调尝试 {selected.callback_attempts} 次
            </p>
            <pre className="whitespace-pre-wrap break-all text-xs leading-6">
              {selected.callback_result || "暂无回调响应"}
            </pre>
          </div>
          {selected.status === "paid" && (
            <Button
              variant="secondary"
              isPending={p.busy}
              onPress={() =>
                p.run(async () => {
                  await api(
                    scoped(
                      "retry/" + selected.trade_no,
                      String(selected.merchant_id),
                    ),
                    "POST",
                    {},
                  );
                  onClose();
                }, "已安排回调重试")
              }
            >
              <RotateCw size={16} />
              重发回调
            </Button>
          )}
        </>
      )}
    </Dialog>
  );
}
export function Events(p: PageProps) {
  const [page, setPage] = useState(1);
  const r = useLoad(
    () =>
      api<{ data: EventRecord[] }>(scoped("events?page=" + page, p.merchant)),
    [p.refresh, p.merchant, page],
  );
  return (
    <Panel
      title="设备通知记录"
      description="原始通知时间用于区分已释放、重新分配的付款金额"
    >
      <Guard {...r}>
        <DataTable
          label="收款通知"
          headers={["通知内容", "收款金额", "匹配结果", "通知 / 上报时间"]}
          rows={(r.data?.data || []).slice(0, 30).map((e) => [
            <div className="max-w-sm">
              <p>{e.raw_text}</p>
              <p className="mt-1 text-xs text-muted">{e.reason}</p>
              {e.trade_no && (
                <p className="mt-1 font-mono text-[10px] text-muted">
                  {e.trade_no}
                </p>
              )}
            </div>,
            money(e.amount),
            <Status value={e.status} />,
            <div className="text-xs">
              {time(e.posted_at)}
              <p className="mt-1 text-muted">{time(e.received_at)}</p>
            </div>,
          ])}
        />
        <Pagination
          page={page}
          more={(r.data?.data.length || 0) > 30}
          onPage={setPage}
        />
      </Guard>
    </Panel>
  );
}
export function Pools(p: PageProps) {
  const [tick, setTick] = useState(0);
  useEffect(() => {
    const timer = setInterval(() => setTick((n) => n + 1), 15000);
    return () => clearInterval(timer);
  }, []);
  const r = useLoad(
    () => api<{ data: Pool[] }>(scoped("pools", p.merchant)),
    [p.refresh, p.merchant, tick],
  );
  return (
    <div className="space-y-6">
      <Notice>
        每个充值金额从
        −¥0.00（不优惠）开始。优先复用最小空位，按需扩容；付款或超时后释放，尾部空位自动收缩。
      </Notice>
      <Guard loading={r.loading && !r.data} error={r.error}>
        {r.data?.data.length ? (
          <div className="grid gap-5 xl:grid-cols-2">
            {r.data.data.map((pool) => {
              const occupied = new Map(pool.slots.map((s) => [s.discount, s]));
              const visible = Math.min(
                Math.max(pool.capacity + 1, 8),
                pool.pool_limit,
                80,
              );
              return (
                <Panel
                  key={pool.amount_cents}
                  title={money(pool.amount_cents) + " 充值池"}
                  description={
                    "容量 " +
                    pool.capacity +
                    " / " +
                    pool.pool_limit +
                    " · 占用 " +
                    pool.occupied +
                    " · 最大优惠 " +
                    money(pool.pool_limit - 1)
                  }
                  action={
                    <Chip
                      variant="soft"
                      color={pool.occupied ? "accent" : "default"}
                    >
                      {pool.occupied ? "使用中" : "空闲"}
                    </Chip>
                  }
                >
                  <div className="pool-grid">
                    {Array.from({ length: visible }, (_, i) => i).map(
                      (slot) => (
                        <div
                          key={slot}
                          className={
                            "pool-slot " +
                            (occupied.has(slot)
                              ? "occupied"
                              : slot < pool.capacity
                                ? "available"
                                : "future")
                          }
                          title={
                            occupied.has(slot)
                              ? occupied.get(slot)!.out_trade_no +
                                " · " +
                                time(occupied.get(slot)!.expires_at)
                              : "可分配：优惠 " + money(slot)
                          }
                        >
                          <span>−{(slot / 100).toFixed(2)}</span>
                          <small>
                            {occupied.has(slot)
                              ? "占用"
                              : slot < pool.capacity
                                ? "空位"
                                : "待扩容"}
                          </small>
                        </div>
                      ),
                    )}
                  </div>
                  {pool.capacity > visible && (
                    <p className="mt-3 text-xs text-muted">
                      还有 {pool.capacity - visible}{" "}
                      个优惠位，完整占用数见上方。
                    </p>
                  )}
                  <div className="mt-5 flex gap-5 text-[10px] text-muted">
                    <span>
                      <i className="legend-dot bg-accent" />
                      已占用
                    </span>
                    <span>
                      <i className="legend-dot border border-accent" />
                      可复用
                    </span>
                    <span>
                      <i className="legend-dot bg-default" />
                      按需扩容
                    </span>
                  </div>
                </Panel>
              );
            })}
          </div>
        ) : (
          <Panel
            title="暂无占用中的金额池"
            description="没有占用的池子会自动隐藏，新订单分配后重新显示。"
          >
            <Empty description="无需手动创建金额池。可在商户配置中设置容量上限。" />
          </Panel>
        )}
      </Guard>
    </div>
  );
}
function networkLabel(n: string, ssid?: string) {
  if (n === "wifi") return ssid ? "Wi-Fi · " + ssid : "Wi-Fi · 名称未获取";
  return (
    (
      {
        wifi: "Wi-Fi",
        cellular: "移动网络",
        ethernet: "以太网",
        vpn: "VPN",
        none: "无网络",
        other: "其他",
        unknown: "未知",
      } as Record<string, string>
    )[n] || n
  );
}
export function Devices(p: PageProps) {
  const [tick, setTick] = useState(0),
    [name, setName] = useState("收款手机"),
    [confirm, setConfirm] = useState(""),
    [credentials, setCredentials] = useState<{
      token: string;
      server_url: string;
    }>();
  useEffect(() => {
    const t = setInterval(() => setTick((n) => n + 1), 15000);
    return () => clearInterval(t);
  }, []);
  const r = useLoad(
    () => api<{ data: Device[] }>(scoped("device", p.merchant)),
    [p.merchant, p.refresh, tick],
  );
  const d = r.data?.data[0];
  useEffect(() => {
    setName(d?.name || "收款手机");
  }, [d?.name, p.merchant]);
  return (
    <div className="space-y-6">
      <Guard loading={r.loading && !r.data} error={r.error}>
        {d ? (
          <Panel
            title={d.name}
            description={
              d.model
                ? d.model + " · Android " + d.android_version
                : "等待 ANE 客户端首次连接"
            }
            action={
              <Status
                value={d.revoked ? "revoked" : d.online ? "online" : "offline"}
              />
            }
          >
            <div className="device-metrics">
              <div>
                <div className="device-metric-icon">
                  {d.charging ? (
                    <BatteryCharging size={24} />
                  ) : (
                    <Battery size={24} />
                  )}
                </div>
                <strong>{d.battery < 0 ? "—" : d.battery + "%"}</strong>
                <span>{d.charging ? "正在充电" : "剩余电量"}</span>
              </div>
              <div>
                <div className="device-metric-icon">
                  <Wifi size={24} />
                </div>
                <strong className="!text-xl">
                  {networkLabel(d.network, d.wifi_ssid)}
                </strong>
                <span>
                  {d.network_validated ? "互联网已验证" : "互联网未验证"}
                </span>
              </div>
              <div>
                <div className="device-metric-icon">
                  <Radio size={24} />
                </div>
                <strong>{d.online ? "在线" : "离线"}</strong>
                <span>3 分钟无心跳判定离线</span>
              </div>
              <div>
                <div className="device-metric-icon">
                  <Activity size={24} />
                </div>
                <strong>{d.pending_events}</strong>
                <span>手机待上报通知</span>
              </div>
            </div>
            <div className="grid gap-6 border-t border-border pt-6 sm:grid-cols-2 xl:grid-cols-4">
              <KV label="最近心跳" value={time(d.last_seen)} />
              <KV
                label="通知监听"
                value={d.listener_connected ? "已连接" : "未连接"}
              />
              <KV
                label="前台服务"
                value={d.service_running ? "运行中" : "未运行"}
              />
              <KV label="客户端版本" value={d.version || "—"} />
              <KV
                label="设备绑定标识"
                value={
                  <span className="font-mono text-xs">
                    {d.installation_id || "首次连接时自动绑定"}
                  </span>
                }
              />
              <KV label="绑定时间" value={time(d.bound_at)} />
              <KV
                label="设备开机时长"
                value={
                  Math.floor(d.uptime_seconds / 3600) +
                  " 小时 " +
                  Math.floor((d.uptime_seconds % 3600) / 60) +
                  " 分钟"
                }
              />
            </div>
          </Panel>
        ) : (
          <Panel
            title="连接你的监听设备"
            description="每位商户仅绑定一台手机，设备标识由 Android 系统提供，复制配置到其他手机不会复制设备绑定。"
          >
            <Empty
              title="尚未配置设备"
              description="下方生成连接令牌，复制到 ANE 即可连接。"
            />
          </Panel>
        )}
      </Guard>
      <Panel
        title={d ? "设备连接管理" : "创建设备连接"}
        description="重置连接会立即使旧令牌失效，并解除旧设备绑定。"
      >
        <div className="flex flex-wrap items-end gap-4">
          <div className="min-w-64 flex-1">
            <Field label="设备名称" value={name} onChange={setName} />
          </div>
          {d && (
            <Button
              isPending={p.busy}
              isDisabled={!name.trim() || name === d.name}
              onPress={() =>
                p.run(async () => {
                  await api(scoped("device", p.merchant), "PATCH", { name });
                }, "设备名称已保存")
              }
            >
              保存名称
            </Button>
          )}
          <Button
            variant="secondary"
            isPending={p.busy}
            onPress={() => setConfirm("reset")}
          >
            <KeyRound size={16} />
            {d ? "重置绑定与令牌" : "生成连接令牌"}
          </Button>
          {d && !d.revoked && (
            <Button variant="danger-soft" onPress={() => setConfirm("revoke")}>
              撤销连接
            </Button>
          )}
        </div>
        <p className="mt-5 text-xs leading-6 text-muted">
          客户端每分钟上报心跳，页面每 15
          秒更新。在线仅代表设备在通信，请同时检查通知监听与前台服务状态。
        </p>
      </Panel>
      <Dialog
        title={confirm === "reset" ? "生成新的设备连接？" : "撤销设备连接？"}
        open={!!confirm}
        onClose={() => setConfirm("")}
        footer={
          <>
            <Button variant="tertiary" onPress={() => setConfirm("")}>
              取消
            </Button>
            <Button
              variant={confirm === "revoke" ? "danger" : "primary"}
              isPending={p.busy}
              onPress={() =>
                p.run(
                  async () => {
                    if (confirm === "reset") {
                      const c = await api<{
                        token: string;
                        server_url: string;
                      }>(scoped("device", p.merchant), "POST", { name });
                      setCredentials(c);
                    } else await api(scoped("device", p.merchant), "DELETE");
                    setConfirm("");
                  },
                  confirm === "reset" ? "设备连接已生成" : "设备连接已撤销",
                )
              }
            >
              确认
            </Button>
          </>
        }
      >
        <p className="text-sm leading-7 text-muted">
          {confirm === "reset"
            ? "ANE 将为当前商户生成一个新令牌。旧连接立即失效，新令牌首次连接的设备会成为唯一绑定设备。"
            : "撤销后手机无法继续发送通知或心跳，待上报通知会保留在手机中。"}
        </p>
      </Dialog>
      <Dialog
        title="保存设备连接信息"
        open={!!credentials}
        onClose={() => setCredentials(undefined)}
      >
        {credentials && (
          <>
            <Notice>令牌仅显示这一次，请复制到 ANE 的连接配置中。</Notice>
            <Field label="服务端地址" value={credentials.server_url} readOnly />
            <Field label="设备令牌" value={credentials.token} readOnly />
            <Button
              onPress={() =>
                p.run(async () => {
                  await navigator.clipboard.writeText(
                    JSON.stringify({
                      server_url: credentials.server_url,
                      device_token: credentials.token,
                    }),
                  );
                }, "连接信息已复制")
              }
            >
              <Copy size={16} />
              复制 JSON 连接信息
            </Button>
          </>
        )}
      </Dialog>
    </div>
  );
}
export function SettingsPage(p: PageProps) {
  const r = useLoad(
    () =>
      api<{ data: Merchant; base_url: string }>(scoped("settings", p.merchant)),
    [p.merchant, p.refresh],
  );
  const [form, setForm] = useState<Merchant>();
  useEffect(() => {
    if (r.data) setForm(r.data.data);
  }, [r.data]);
  const set = (v: Partial<Merchant>) =>
    setForm((f) => (f ? { ...f, ...v } : f));
  return (
    <Guard loading={r.loading && !form} error={r.error}>
      {form && (
        <>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              p.run(async () => {
                await api(scoped("settings", p.merchant), "PUT", {
                  name: form.name,
                  qr_image: form.qr_image || "",
                  pool_limit: form.pool_limit,
                  order_ttl_minutes: form.order_ttl_minutes,
                  payment_note: form.payment_note,
                });
              });
            }}
          >
            <div className="grid gap-6 xl:grid-cols-2">
              <Panel
                title="收款配置"
                description="使用允许付款人输入金额的支付宝收款码"
              >
                <Field
                  label="商户名称"
                  value={form.name}
                  onChange={(v) => set({ name: v })}
                  required
                />
                <div className="qr-upload">
                  {form.qr_image ? (
                    <img src={form.qr_image} alt="支付宝收款码" />
                  ) : (
                    <div className="qr-placeholder">
                      <Upload size={28} />
                      <span>上传你的收款码</span>
                    </div>
                  )}
                  <div>
                    <label className="file-button">
                      <Upload size={15} />
                      选择图片
                      <input
                        type="file"
                        accept="image/png,image/jpeg"
                        onChange={(e) => {
                          const f = e.target.files?.[0];
                          if (!f) return;
                          if (f.size > 2 * 1024 * 1024) {
                            p.notify("图片不能超过 2 MiB");
                            return;
                          }
                          const reader = new FileReader();
                          reader.onload = () =>
                            set({ qr_image: String(reader.result) });
                          reader.onerror = () => p.notify("读取图片失败");
                          reader.readAsDataURL(f);
                        }}
                      />
                    </label>
                    <p className="mt-2 text-[11px] text-muted">
                      PNG / JPEG，最大 2 MiB
                    </p>
                  </div>
                </div>
                <p className="text-xs leading-6 text-muted">
                  新收款码用于后续订单。已创建订单保留当时的收款码，避免付款过程中切换收款账户。
                </p>
              </Panel>
              <Panel
                title="优惠金额池"
                description="每个充值金额独立管理，优先分配最小可用优惠"
              >
                <div className="space-y-5">
                  <Field
                    label="最大容量"
                    type="number"
                    value={String(form.pool_limit)}
                    onChange={(v) => set({ pool_limit: Number(v) })}
                    min={1}
                    max={9999}
                    required
                    hint={
                      "每个池子最多 " +
                      form.pool_limit +
                      " 个位置（含原价位）；最大让利 " +
                      money(Math.max(0, form.pool_limit - 1)) +
                      "。"
                    }
                  />
                  <Field
                    label="付款有效期（分钟）"
                    type="number"
                    value={String(form.order_ttl_minutes)}
                    onChange={(v) => set({ order_ttl_minutes: Number(v) })}
                    min={1}
                    max={60}
                    required
                  />
                  <div className="pool-example">
                    <span>订单 ¥10.00</span>
                    <ArrowRight size={15} />
                    <span>实付 ¥10.00</span>
                    <Chip size="sm" color="success">
                      不优惠
                    </Chip>
                  </div>
                  <p className="text-xs leading-6 text-muted">
                    原价位置被占用时分配 ¥9.99，付款或超时后优先复用
                    ¥10.00。缩小上限不会取消已有订单，后续分配遵循新上限。
                  </p>
                </div>
              </Panel>
            </div>
            <div className="mt-5 flex justify-end">
              <Button type="submit" isPending={p.busy}>
                保存商户配置
                <Check size={16} />
              </Button>
            </div>
          </form>
        </>
      )}
    </Guard>
  );
}
export function Merchants(p: PageProps) {
  const r = useLoad(() => api<{ data: Merchant[] }>("merchants"), [p.refresh]);
  const [create, setCreate] = useState(false),
    [edit, setEdit] = useState<Merchant>(),
    [password, setPassword] = useState(""),
    [newM, setNewM] = useState({
      username: "",
      name: "",
      password: "",
      pool_limit: 99,
      order_ttl_minutes: 10,
    });
  return (
    <>
      <Panel
        title="所有商户"
        description="商户拥有独立登录、密钥、收款码、金额池和监听设备"
        action={
          <Button onPress={() => setCreate(true)}>
            <Plus size={16} />
            创建商户
          </Button>
        }
      >
        <Guard {...r}>
          <DataTable
            label="商户列表"
            headers={[
              "商户",
              "登录账号",
              "角色 / 状态",
              "应用个数",
              "池上限",
              "设备",
              "操作",
            ]}
            rows={(r.data?.data || []).map((m) => [
              <div className="flex items-center gap-3">
                <div className="merchant-avatar">{m.name.slice(0, 1)}</div>
                <div className="font-medium">
                  {m.name}
                  <p className="text-xs text-muted">#{m.id}</p>
                </div>
              </div>,
              m.username,
              <div className="flex gap-2">
                <Chip size="sm" variant="soft">
                  {m.is_admin ? "管理员" : "商户"}
                </Chip>
                <Status value={m.enabled ? "enabled" : "disabled"} />
              </div>,
              m.app_count || 0,
              <span>
                {m.pool_limit} 位{" "}
                <small className="text-muted">
                  / {money(m.pool_limit - 1)}
                </small>
              </span>,
              <Status value={m.device_online ? "online" : "offline"} />,
              <Button
                size="sm"
                variant="ghost"
                onPress={() => {
                  setEdit(m);
                  setPassword("");
                }}
              >
                管理
                <ArrowUpRight size={14} />
              </Button>,
            ])}
          />
        </Guard>
      </Panel>
      <Dialog
        title="创建商户"
        open={create}
        onClose={() => setCreate(false)}
        footer={
          <>
            <Button variant="tertiary" onPress={() => setCreate(false)}>
              取消
            </Button>
            <Button
              isPending={p.busy}
              onPress={() =>
                p.run(async () => {
                  await api("merchants", "POST", newM);
                  setCreate(false);
                  setNewM({
                    username: "",
                    name: "",
                    password: "",
                    pool_limit: 99,
                    order_ttl_minutes: 10,
                  });
                }, "商户已创建")
              }
            >
              创建商户
            </Button>
          </>
        }
      >
        <Field
          label="商户名称"
          value={newM.name}
          onChange={(v) => setNewM({ ...newM, name: v })}
        />
        <Field
          label="登录账号"
          value={newM.username}
          onChange={(v) => setNewM({ ...newM, username: v })}
          hint="3–40 位小写字母、数字、下划线或连字符"
        />
        <Field
          label="登录密码"
          type="password"
          value={newM.password}
          onChange={(v) => setNewM({ ...newM, password: v })}
          hint="16–72 字节"
        />
        <div className="grid grid-cols-2 gap-4">
          <Field
            label="池最大容量"
            type="number"
            value={String(newM.pool_limit)}
            onChange={(v) => setNewM({ ...newM, pool_limit: Number(v) })}
          />
          <Field
            label="有效期（分钟）"
            type="number"
            value={String(newM.order_ttl_minutes)}
            onChange={(v) => setNewM({ ...newM, order_ttl_minutes: Number(v) })}
          />
        </div>
      </Dialog>
      <Dialog
        title="管理商户"
        open={!!edit}
        onClose={() => setEdit(undefined)}
        footer={
          <>
            <Button variant="tertiary" onPress={() => setEdit(undefined)}>
              取消
            </Button>
            <Button
              isPending={p.busy}
              onPress={() =>
                p.run(async () => {
                  if (edit) {
                    await api("merchants/" + edit.id, "PATCH", {
                      name: edit.name,
                      enabled: edit.enabled,
                      password,
                    });
                    setEdit(undefined);
                  }
                }, "商户已更新")
              }
            >
              保存更改
            </Button>
          </>
        }
      >
        {edit && (
          <>
            <Field
              label="商户名称"
              value={edit.name}
              onChange={(v) => setEdit({ ...edit, name: v })}
            />
            <Field label="登录账号" value={edit.username} readOnly />
            {edit.is_admin ? (
              <Notice>管理员登录账号和密码由 env 配置。</Notice>
            ) : (
              <>
                <Toggle
                  label="启用商户"
                  description="停用会阻止登录、新订单和设备上报。已支付订单的回调继续处理。"
                  selected={edit.enabled}
                  onChange={(v) => setEdit({ ...edit, enabled: v })}
                />
                <Field
                  label="重置密码"
                  type="password"
                  value={password}
                  onChange={setPassword}
                  hint="留空保留原密码。重置后现有登录会话失效。"
                />
              </>
            )}
          </>
        )}
      </Dialog>
    </>
  );
}
export function SMTPPage(p: PageProps) {
  const r = useLoad(
    () => api<{ data: SMTP; has_password: boolean }>("smtp"),
    [p.refresh],
  );
  const [form, setForm] = useState<SMTP>(),
    [clear, setClear] = useState(false),
    [testOpen, setTestOpen] = useState(false),
    [testEmail, setTestEmail] = useState(""),
    [testBusy, setTestBusy] = useState(false),
    [testError, setTestError] = useState("");
  useEffect(() => {
    if (r.data) {
      setForm({ ...r.data.data, password: "" });
      setClear(false);
    }
  }, [r.data]);
  const set = (v: Partial<SMTP>) => setForm((f) => (f ? { ...f, ...v } : f));
  return (
    <Guard loading={r.loading && !form} error={r.error}>
      {form && (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            p.run(async () => {
              await api("smtp", "PUT", { ...form, clear_password: clear });
            });
          }}
        >
          <Panel
            title="SMTP 发送服务"
            description="全局邮件服务，仅管理员可配置。凭据保存于数据库，API 不回显密码。"
          >
            <div className="max-w-3xl space-y-7">
              <Toggle
                label="启用邮件发送"
                description="关闭时保留通知队列，重新启用后继续发送仍有效的订阅。"
                selected={form.enabled}
                onChange={(v) => set({ enabled: v })}
              />
              <div className="grid gap-5 sm:grid-cols-2">
                <Field
                  label="SMTP 主机"
                  value={form.host}
                  onChange={(v) => set({ host: v })}
                  placeholder="smtp.example.com"
                />
                <Field
                  label="端口"
                  type="number"
                  min={1}
                  max={65535}
                  value={String(form.port)}
                  onChange={(v) => set({ port: Number(v) })}
                />
                <Choice
                  label="连接安全"
                  value={form.security}
                  onChange={(v) => set({ security: v })}
                  options={[
                    { id: "starttls", label: "STARTTLS · 通常 587" },
                    { id: "tls", label: "TLS · 通常 465" },
                    { id: "none", label: "无加密 · 仅可信内部中继" },
                  ]}
                />
                <Field
                  label="SMTP 用户名"
                  value={form.username}
                  onChange={(v) => set({ username: v })}
                />
                <div className="sm:col-span-2">
                  <Field
                    label="SMTP 密码 / 授权码"
                    type="password"
                    value={form.password || ""}
                    onChange={(v) => set({ password: v })}
                    hint={
                      r.data?.has_password
                        ? "已保存密码，留空保留。"
                        : "尚未设置密码。"
                    }
                  />
                </div>
                <Field
                  label="发件邮箱"
                  type="email"
                  value={form.from_address}
                  onChange={(v) => set({ from_address: v })}
                />
                <Field
                  label="发件人名称"
                  value={form.from_name}
                  onChange={(v) => set({ from_name: v })}
                />
              </div>
              {r.data?.has_password && (
                <Toggle
                  label="清除已保存密码"
                  selected={clear}
                  onChange={setClear}
                />
              )}
              <Notice>
                选择无加密时不允许发送 SMTP 登录凭据。TLS / STARTTLS
                会校验证书，避免将授权码发送给错误服务器。
              </Notice>
              <div className="flex flex-wrap gap-3">
                <Button type="submit" isPending={p.busy}>
                  保存邮件服务
                  <Mail size={16} />
                </Button>
                <Button
                  type="button"
                  variant="secondary"
                  onPress={() => {
                    setTestError("");
                    setTestOpen(true);
                  }}
                >
                  发送测试邮件
                </Button>
              </div>
            </div>
          </Panel>
        </form>
      )}
      <Dialog
        title="发送测试邮件"
        open={testOpen}
        onClose={() => {
          if (!testBusy) setTestOpen(false);
        }}
        footer={
          <>
            <Button
              variant="tertiary"
              isDisabled={testBusy}
              onPress={() => setTestOpen(false)}
            >
              取消
            </Button>
            <Button type="submit" form="smtp-test-form" isPending={testBusy}>
              确定发送
            </Button>
          </>
        }
      >
        <form
          id="smtp-test-form"
          className="space-y-4"
          onSubmit={async (e) => {
            e.preventDefault();
            setTestBusy(true);
            setTestError("");
            try {
              await api("smtp/test", "POST", { email: testEmail });
              setTestOpen(false);
              p.notify("测试邮件已发送，请检查收件箱");
            } catch (e) {
              setTestError((e as Error).message);
            } finally {
              setTestBusy(false);
            }
          }}
        >
          <p className="text-sm text-muted">
            使用已保存的 SMTP 配置发送，请先保存配置更改。
          </p>
          <Field
            label="接收邮箱"
            type="email"
            value={testEmail}
            onChange={setTestEmail}
            required
            placeholder="you@example.com"
          />
          {testError && <Notice error>{testError}</Notice>}
        </form>
      </Dialog>
    </Guard>
  );
}
