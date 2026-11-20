import { useEffect, useState } from "react";
import {
  Button,
  Chip,
  Label,
  TextArea,
  TextField,
  Tooltip,
} from "@heroui/react";
import { Copy, Plus, RotateCcw, Search } from "lucide-react";
import { api, maskAppKey, scoped, time } from "./api";
import {
  Choice,
  DataTable,
  Dialog,
  Field,
  Notice,
  Pagination,
  Panel,
  SkeletonTable,
  Status,
  Toggle,
  useLoad,
} from "./components";
import { TimeRange, MultiChoice } from "./DateTime";
import type { PageProps } from "./App";

interface AppRecord {
  name: string;
  merchant_key: string;
  enabled: boolean;
  payment_note: string;
  emails: { id: number; email: string }[];
}

export function Applications(p: PageProps) {
  const r = useLoad(
    () => api<{ data: AppRecord[] }>(scoped("apps", p.merchant)),
    [p.merchant, p.refresh],
  );
  const [create, setCreate] = useState(false),
    [name, setName] = useState(""),
    [error, setError] = useState(""),
    [edit, setEdit] = useState<AppRecord>(),
    [confirm, setConfirm] = useState<{
      app: AppRecord;
      action: "disable" | "delete" | "rotate";
    }>();

  const copy = async (key: string, label = "密钥") => {
    try {
      await navigator.clipboard.writeText(key);
      p.notify(label + "已复制");
    } catch {
      p.notify("复制失败，请重试");
    }
  };

  const act = () =>
    p.run(async () => {
      if (!confirm) return;
      const { app, action } = confirm;
      const result = await api<{ merchant_key: string }>(
        scoped("apps", p.merchant),
        action === "delete" ? "DELETE" : "PATCH",
        {
          name: app.name,
          ...(action === "disable"
            ? { enabled: false }
            : action === "rotate"
              ? { rotate: true }
              : {}),
        },
      );
      setConfirm(undefined);
      if (action === "rotate") await copy(result.merchant_key);
    }, "操作完成");

  return (
    <div className="space-y-5">
      <Panel
        title="易支付接入参数"
        description="各应用使用同一商户 ID 和独立密钥，共享收款设备与优惠池。"
      >
        <div className="grid gap-4 sm:grid-cols-2">
          {[
            { label: "网关地址", value: p.baseURL },
            { label: "商户 ID", value: p.merchant },
          ].map((item) => (
            <div key={item.label} className="flex items-end gap-2">
              <Field label={item.label} value={item.value} readOnly />
              <Tooltip delay={300}>
                <Button
                  variant="secondary"
                  isIconOnly
                  aria-label={"复制" + item.label}
                  onPress={() => copy(item.value, item.label)}
                >
                  <Copy size={15} />
                </Button>
                <Tooltip.Content placement="top">
                  复制{item.label}
                </Tooltip.Content>
              </Tooltip>
            </div>
          ))}
        </div>
      </Panel>

      <Panel
        title="应用管理"
        description="点击密钥复制完整内容；付款页文案随新订单保存。"
        action={
          <Button
            size="sm"
            onPress={() => {
              setName("");
              setError("");
              setCreate(true);
            }}
          >
            <Plus size={15} />
            添加应用
          </Button>
        }
      >
        {r.error && (
          <div className="mb-4">
            <Notice error>{r.error}</Notice>
          </div>
        )}
        {r.loading ? (
          <SkeletonTable rows={4} />
        ) : (
          <DataTable
            label="应用管理"
            emptyTitle="暂无接入应用"
            emptyDescription="点击右上角添加应用以生成独立的易支付密钥。"
            headers={[
              "应用名",
              "密钥",
              "通知邮箱",
              "付款页文案",
              "状态",
              "操作",
            ]}
            rows={(r.data?.data || []).map((app) => [
              <span className="font-semibold text-foreground">{app.name}</span>,
              <Button
                size="sm"
                variant="secondary"
                className="h-7 gap-1.5 px-2.5 font-mono text-xs"
                aria-label={"复制 " + app.name + " 密钥"}
                onPress={() => copy(app.merchant_key)}
              >
                {maskAppKey(app.merchant_key, app.name)}
                <Copy size={12} />
              </Button>,
              <div className="flex flex-wrap gap-1.5">
                {app.emails.length ? (
                  app.emails.map((email) => (
                    <Button
                      key={email.id}
                      size="sm"
                      variant="ghost"
                      className="h-6 px-2 text-xs text-accent-soft-foreground"
                      onPress={() => p.navigate("subscriptions/" + email.id)}
                    >
                      {email.email}
                    </Button>
                  ))
                ) : (
                  <span className="text-muted">—</span>
                )}
              </div>,
              <span
                className="block max-w-48 truncate text-xs text-muted"
                title={app.payment_note}
              >
                {app.payment_note || "—"}
              </span>,
              <Status value={app.enabled ? "enabled" : "disabled"} />,
              <div className="flex flex-wrap gap-1.5">
                <Button
                  size="sm"
                  variant="tertiary"
                  onPress={() => setConfirm({ app, action: "rotate" })}
                >
                  轮换密钥
                </Button>
                <Button
                  size="sm"
                  variant="secondary"
                  onPress={() => setEdit({ ...app })}
                >
                  编辑
                </Button>
                <Button
                  size="sm"
                  variant={app.enabled ? "danger-soft" : "secondary"}
                  isDisabled={p.busy}
                  onPress={() =>
                    app.enabled
                      ? setConfirm({ app, action: "disable" })
                      : p.run(async () => {
                          await api(scoped("apps", p.merchant), "PATCH", {
                            name: app.name,
                            enabled: true,
                          });
                        }, "应用已启用")
                  }
                >
                  {app.enabled ? "禁用" : "启用"}
                </Button>
                <Button
                  size="sm"
                  variant="danger-soft"
                  onPress={() => setConfirm({ app, action: "delete" })}
                >
                  删除
                </Button>
              </div>,
            ])}
          />
        )}
      </Panel>

      <Dialog
        title="添加应用"
        open={create}
        onClose={() => setCreate(false)}
        className="sm:max-w-sm"
        footer={
          <>
            <Button variant="tertiary" onPress={() => setCreate(false)}>
              取消
            </Button>
            <Button
              isPending={p.busy}
              onPress={() =>
                p.run(async () => {
                  if (!/^[A-Za-z0-9_]{1,64}$/.test(name)) {
                    setError("应用名只能包含英文字母、数字和下划线，最多64位");
                    return;
                  }
                  try {
                    const result = await api<{ merchant_key: string }>(
                      scoped("apps", p.merchant),
                      "POST",
                      { name },
                    );
                    setCreate(false);
                    await copy(result.merchant_key);
                  } catch (e) {
                    setError((e as Error).message);
                  }
                }, "")
              }
            >
              创建并复制密钥
            </Button>
          </>
        }
      >
        <Field
          label="应用名"
          value={name}
          onChange={setName}
          placeholder="例如 my_shop"
          hint="仅支持英文字母、数字和下划线"
        />
        {error && <Notice error>{error}</Notice>}
      </Dialog>

      <Dialog
        title="编辑应用"
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
                  await api(scoped("apps", p.merchant), "PATCH", {
                    name: edit!.name,
                    payment_note: edit!.payment_note,
                  });
                  setEdit(undefined);
                })
              }
            >
              保存
            </Button>
          </>
        }
      >
        {edit && (
          <>
            <Field label="应用名" value={edit.name} readOnly />
            <TextField
              className="w-full"
              value={edit.payment_note}
              onChange={(payment_note) => setEdit({ ...edit, payment_note })}
            >
              <Label>付款页文案</Label>
              <TextArea rows={6} maxLength={8000} />
            </TextField>
            <p className="text-xs text-muted">
              支持基础 HTML、颜色和字号。修改仅对新订单生效。
            </p>
          </>
        )}
      </Dialog>

      <Dialog
        title={
          confirm?.action === "delete"
            ? "删除应用？"
            : confirm?.action === "disable"
              ? "禁用应用？"
              : "轮换密钥？"
        }
        open={!!confirm}
        onClose={() => setConfirm(undefined)}
        footer={
          <>
            <Button variant="tertiary" onPress={() => setConfirm(undefined)}>
              取消
            </Button>
            <Button variant="danger" isPending={p.busy} onPress={act}>
              确认
              {confirm?.action === "delete"
                ? "删除"
                : confirm?.action === "disable"
                  ? "禁用"
                  : "轮换并复制"}
            </Button>
          </>
        }
      >
        <p className="text-sm font-medium">应用：{confirm?.app.name}</p>
        <p className="text-sm leading-6 text-muted">
          {confirm?.action === "rotate"
            ? "轮换后请及时更新应用端密钥。"
            : confirm?.action === "delete"
              ? "删除后停止新接入，应用名保留在历史记录中，不能重新使用。"
              : "禁用后停止新订单和查询。"}
          历史订单与原密钥回调保留。
        </p>
      </Dialog>
    </div>
  );
}

interface Subscription {
  id: number;
  email: string;
  topics: string[];
  apps: string[];
}

interface SubscriptionData {
  data: Subscription[];
  available_topics: Record<string, string>;
  smtp_enabled: boolean;
}

interface MailRecord {
  id: number;
  subject: string;
  recipient: string;
  merchant_username: string;
  status: string;
  attempts: number;
  last_error: string;
  created_at: string;
}

export function Subscriptions(p: PageProps) {
  const r = useLoad(
    () => api<SubscriptionData>(scoped("subscriptions", p.merchant)),
    [p.merchant, p.refresh],
  );
  const apps = useLoad(
    () => api<{ data: AppRecord[] }>(scoped("apps", p.merchant)),
    [p.merchant, p.refresh],
  );
  const [edit, setEdit] = useState<Subscription>(),
    [deleting, setDeleting] = useState<Subscription>();
  const [page, setPage] = useState(1),
    [status, setStatus] = useState("all"),
    [topic, setTopic] = useState("all"),
    [query, setQuery] = useState(""),
    [search, setSearch] = useState(""),
    [start, setStart] = useState(""),
    [end, setEnd] = useState("");

  useEffect(() => {
    const open = () => {
      const match = window.location.hash.match(/^#\/subscriptions\/(\d+)/);
      if (match && r.data) {
        const row = r.data.data.find((s) => s.id === Number(match[1]));
        if (row) setEdit({ ...row });
      } else setEdit(undefined);
    };
    open();
    window.addEventListener("hashchange", open);
    return () => window.removeEventListener("hashchange", open);
  }, [r.data]);

  const close = () => {
    setEdit(undefined);
    if (window.location.hash.startsWith("#/subscriptions/"))
      p.navigate("subscriptions");
  };

  const mails = useLoad(() => {
    const params = new URLSearchParams({ page: String(page), q: search });
    if (status !== "all") params.set("status", status);
    if (topic !== "all") params.set("topic", topic);
    if (start) params.set("start", start);
    if (end) params.set("end", end);
    return api<{ data: MailRecord[]; has_more: boolean }>(
      scoped("mail?" + params, p.merchant),
    );
  }, [p.merchant, p.refresh, page, status, topic, search, start, end]);

  return (
    <div className="space-y-5">
      {r.error && <Notice error>{r.error}</Notice>}
      {r.data && !r.data.smtp_enabled && (
        <Notice>邮件服务尚未启用。可先配置订阅，再由管理员启用 SMTP。</Notice>
      )}

      <Panel
        title="邮箱订阅"
        description="每个邮箱独立选择事件和应用。设备及未匹配通知属于商户级事件，不受应用选择限制。"
        action={
          <Button
            size="sm"
            onPress={() => setEdit({ id: 0, email: "", topics: [], apps: [] })}
          >
            <Plus size={15} />
            添加邮箱
          </Button>
        }
      >
        {r.loading ? (
          <SkeletonTable rows={3} />
        ) : (
          <DataTable
            label="邮箱订阅"
            emptyTitle="暂无订阅邮箱"
            emptyDescription="添加接收邮箱后，系统会在关键收款事件发生时自动推送邮件提醒。"
            headers={["接收邮箱", "订阅事件", "关联应用", "操作"]}
            rows={(r.data?.data || []).map((s) => [
              <span className="font-medium text-foreground">{s.email}</span>,
              <div className="flex flex-wrap gap-1.5">
                {s.topics.length ? (
                  s.topics.map((t) => (
                    <Chip key={t} size="sm" variant="soft" color="accent">
                      {r.data?.available_topics[t] || t}
                    </Chip>
                  ))
                ) : (
                  <span className="text-xs text-muted">未订阅</span>
                )}
              </div>,
              <div className="flex flex-wrap gap-1.5">
                {s.apps.length ? (
                  s.apps.map((a) => (
                    <Chip key={a} size="sm" variant="soft">
                      {a}
                    </Chip>
                  ))
                ) : (
                  <span className="text-xs text-muted">无应用</span>
                )}
              </div>,
              <div className="flex gap-1.5">
                <Button
                  size="sm"
                  variant="secondary"
                  onPress={() => p.navigate("subscriptions/" + s.id)}
                >
                  编辑
                </Button>
                <Button
                  size="sm"
                  variant="danger-soft"
                  onPress={() => setDeleting(s)}
                >
                  删除
                </Button>
              </div>,
            ])}
          />
        )}
      </Panel>

      <Dialog
        title={edit?.id ? "编辑邮箱订阅" : "添加邮箱订阅"}
        open={!!edit}
        onClose={close}
        footer={
          <>
            <Button variant="tertiary" onPress={close}>
              取消
            </Button>
            <Button
              isPending={p.busy}
              onPress={() =>
                p.run(async () => {
                  await api<{ id: number }>(
                    scoped("subscriptions", p.merchant),
                    edit!.id ? "PUT" : "POST",
                    edit,
                  );
                  close();
                })
              }
            >
              保存订阅
            </Button>
          </>
        }
      >
        {edit && (
          <>
            <Field
              label="接收邮箱"
              type="email"
              value={edit.email}
              onChange={(email) => setEdit({ ...edit, email })}
              placeholder="ops@example.com"
            />
            <MultiChoice
              label="订阅的应用"
              value={edit.apps}
              onChange={(values) => setEdit({ ...edit, apps: values })}
              options={(apps.data?.data || []).map((app) => ({
                id: app.name,
                label: app.name,
              }))}
            />
            <div className="space-y-2.5">
              {Object.entries(r.data?.available_topics || {}).map(
                ([key, label]) => (
                  <Toggle
                    key={key}
                    label={label}
                    selected={edit.topics.includes(key)}
                    onChange={(v) =>
                      setEdit({
                        ...edit,
                        topics: v
                          ? [...edit.topics, key]
                          : edit.topics.filter((t) => t !== key),
                      })
                    }
                  />
                ),
              )}
            </div>
          </>
        )}
      </Dialog>

      <Dialog
        title="删除邮箱订阅？"
        open={!!deleting}
        onClose={() => setDeleting(undefined)}
        footer={
          <>
            <Button variant="tertiary" onPress={() => setDeleting(undefined)}>
              取消
            </Button>
            <Button
              variant="danger"
              isPending={p.busy}
              onPress={() =>
                p.run(async () => {
                  await api(scoped("subscriptions", p.merchant), "DELETE", {
                    id: deleting!.id,
                  });
                  setDeleting(undefined);
                }, "订阅已删除")
              }
            >
              确认删除
            </Button>
          </>
        }
      >
        <p className="font-medium">{deleting?.email}</p>
        <p className="text-sm text-muted">
          该邮箱尚未发送的邮件将取消，历史发送记录保留。
        </p>
      </Dialog>

      <Panel
        title="邮件发送记录"
        description="按创建时间筛选，时间使用当前浏览器时区。"
        action={
          <Button
            size="sm"
            variant="tertiary"
            onPress={() => {
              setStart("");
              setEnd("");
              setStatus("all");
              setTopic("all");
              setQuery("");
              setSearch("");
              setPage(1);
            }}
          >
            <RotateCcw size={14} />
            重置筛选
          </Button>
        }
      >
        <div className="filter-bar">
          <div className="grid items-end gap-3 md:grid-cols-2 xl:grid-cols-4">
            <Choice
              label="状态"
              value={status}
              onChange={(v) => {
                setStatus(v);
                setPage(1);
              }}
              options={[
                { id: "all", label: "全部状态" },
                { id: "pending", label: "待发送" },
                { id: "retrying", label: "重试中" },
                { id: "sent", label: "已发送" },
                { id: "failed", label: "失败" },
                { id: "cancelled", label: "已取消" },
              ]}
            />
            <Choice
              label="通知类型"
              value={topic}
              onChange={(v) => {
                setTopic(v);
                setPage(1);
              }}
              options={[
                { id: "all", label: "全部类型" },
                ...Object.entries(r.data?.available_topics || {}).map(
                  ([id, label]) => ({ id, label }),
                ),
              ]}
            />
            <form
              className="flex items-end gap-2"
              onSubmit={(e) => {
                e.preventDefault();
                setSearch(query);
                setPage(1);
              }}
            >
              <Field
                label="搜索"
                value={query}
                onChange={setQuery}
                placeholder="主题或邮箱"
              />
              <Button
                type="submit"
                isIconOnly
                variant="secondary"
                aria-label="搜索邮件"
              >
                <Search size={16} />
              </Button>
            </form>
            <TimeRange
              label="创建时间"
              start={start}
              end={end}
              onChange={(from, to) => {
                setStart(from);
                setEnd(to);
                setPage(1);
              }}
            />
          </div>
        </div>

        {mails.error && (
          <div className="mb-4">
            <Notice error>{mails.error}</Notice>
          </div>
        )}
        {mails.loading ? (
          <SkeletonTable rows={5} />
        ) : (
          <DataTable
            label="邮件记录"
            headers={[
              "通知",
              "商户账号",
              "接收邮箱",
              "状态",
              "发送尝试",
              "时间",
            ]}
            rows={(mails.data?.data || []).map((m) => [
              <div>
                <div className="font-medium text-foreground">{m.subject}</div>
                {m.last_error && (
                  <p className="mt-1 max-w-sm break-all text-xs text-danger">
                    {m.last_error}
                  </p>
                )}
              </div>,
              <span className="font-mono text-xs">{m.merchant_username}</span>,
              m.recipient,
              <Status value={m.status} />,
              <span className="tabular-nums">{m.attempts}</span>,
              <span className="text-xs text-muted tabular-nums">
                {time(m.created_at)}
              </span>,
            ])}
          />
        )}
        <Pagination
          page={page}
          more={mails.data?.has_more || false}
          onPage={setPage}
        />
      </Panel>
    </div>
  );
}
