import { useCallback, useEffect, useState } from "react";
import {
  Avatar,
  Breadcrumbs,
  Button,
  Card,
  Chip,
  Input,
  Label,
  TextField,
  Tooltip,
  toast,
} from "@heroui/react";
import {
  Activity,
  AppWindow,
  ArrowRight,
  CircleDollarSign,
  Layers,
  LayoutDashboard,
  LogOut,
  Mail,
  Menu,
  Moon,
  Radio,
  RefreshCw,
  Settings,
  ShieldCheck,
  Smartphone,
  Sparkles,
  Sun,
  Users,
} from "lucide-react";
import { api, type Merchant } from "./api";
import { Choice, Loading, Notice } from "./components";
import {
  Overview,
  Orders,
  Events,
  Pools,
  Devices,
  SettingsPage,
  Merchants,
  SMTPPage,
} from "./pages";
import { Applications, Subscriptions } from "./Management";
import { useThemeMode } from "./theme";

export type PageProps = {
  navigate: (view: string) => void;
  merchant: string;
  baseURL: string;
  refresh: number;
  user: Merchant;
  reload: () => void;
  notify: (text: string) => void;
  run: (fn: () => Promise<void>, message?: string) => Promise<void>;
  busy: boolean;
};

const navigation = [
  {
    id: "overview",
    name: "工作台",
    icon: LayoutDashboard,
    section: "收款业务",
  },
  { id: "orders", name: "交易订单", icon: CircleDollarSign },
  { id: "events", name: "收款通知", icon: Activity },
  { id: "pools", name: "优惠金额池", icon: Layers },
  { id: "device", name: "监听设备", icon: Smartphone },
  { id: "apps", name: "易支付应用", icon: AppWindow },
  { id: "settings", name: "商户配置", icon: Settings },
  { id: "subscriptions", name: "通知订阅", icon: Mail, section: "账户与管理" },
  { id: "merchants", name: "商户管理", icon: Users, admin: true },
  { id: "smtp", name: "邮件服务", icon: ShieldCheck, admin: true },
];

const descriptions: Record<string, string> = {
  overview: "每一笔收款，每一个连接，一目了然。",
  orders: "从订单创建到回调送达，跟踪交易的每一步。",
  events: "查看设备捕获的通知及自动匹配结果。",
  pools: "以最小优惠区分订单，让每一分钱都恰到好处。",
  device: "一位商户，一台设备。持续掌握收款端的运行状态。",
  settings: "收款方式与优惠策略，集中配置。",
  apps: "管理易支付应用、密钥与独立付款页文案。",
  merchants: "创建和管理独立的商户工作空间。",
  subscriptions: "选择你关心的动态，及时接收邮件提醒。",
  smtp: "连接邮件服务，为商户提供可靠的通知。",
};

function ThemeToggleButton({
  resolved,
  onToggle,
}: {
  resolved: "light" | "dark";
  onToggle: () => void;
}) {
  const label = resolved === "dark" ? "切换到浅色模式" : "切换到深色模式";
  return (
    <Tooltip delay={350}>
      <Button
        isIconOnly
        size="sm"
        variant="ghost"
        aria-label={label}
        onPress={onToggle}
      >
        {resolved === "dark" ? <Sun size={16} /> : <Moon size={16} />}
      </Button>
      <Tooltip.Content placement="bottom">{label}</Tooltip.Content>
    </Tooltip>
  );
}

export function App() {
  const { resolved: theme, cycleMode } = useThemeMode();
  const [baseURL, setBaseURL] = useState(window.location.origin);
  const [user, setUser] = useState<Merchant | null>(null),
    [starting, setStarting] = useState(true),
    [merchants, setMerchants] = useState<Merchant[]>([]),
    [merchant, setMerchant] = useState(""),
    [view, setView] = useState(
      () => window.location.hash.slice(2).split(/[/?]/)[0] || "overview",
    ),
    [refresh, setRefresh] = useState(0),
    [spinning, setSpinning] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [mobile, setMobile] = useState(false),
    [connected, setConnected] = useState<boolean | null>(null);

  const notify = useCallback((text: string) => {
    if (text) toast.success(text);
  }, []);

  const reload = useCallback(() => {
    setRefresh((n) => n + 1);
    setSpinning(true);
    setTimeout(() => setSpinning(false), 600);
  }, []);

  const run = useCallback(
    async (fn: () => Promise<void>, message = "已保存") => {
      setBusy(true);
      setError("");
      try {
        await fn();
        if (message) toast.success(message);
        setRefresh((n) => n + 1);
      } catch (e) {
        const msg = (e as Error).message;
        setError(msg);
        toast.danger(msg);
      } finally {
        setBusy(false);
      }
    },
    [],
  );

  const loadMe = useCallback(async () => {
    const d = await api<{ user: Merchant; base_url: string }>("me");
    setUser(d.user);
    setBaseURL(d.base_url || window.location.origin);
    const raw = new URLSearchParams(
      window.location.hash.split("?")[1] || "",
    ).get("merchant_id");
    setMerchant(raw && d.user.is_admin ? raw : String(d.user.id));
    setError("");
    setMerchants([]);
  }, []);

  useEffect(() => {
    loadMe()
      .catch(() => {})
      .finally(() => setStarting(false));
    const out = () => setUser(null);
    window.addEventListener("ane:logout", out);
    return () => window.removeEventListener("ane:logout", out);
  }, [loadMe]);

  useEffect(() => {
    if (user?.is_admin)
      api<{ data: Merchant[] }>("merchants")
        .then((d) => setMerchants(d.data))
        .catch((e) => setError(e.message));
    else if (user) setMerchants([user]);
  }, [user, refresh]);

  useEffect(() => {
    if (!user) return;
    let active = true;
    const check = async () => {
      try {
        const response = await fetch("/healthz", {
          signal: AbortSignal.timeout(8000),
        });
        if (active) setConnected(response.ok);
      } catch {
        if (active) setConnected(false);
      }
    };
    check();
    const timer = setInterval(check, 30000);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, [user, refresh]);

  const navigate = (path: string) => {
    window.location.hash =
      "/" +
      path +
      (merchant ? "?merchant_id=" + encodeURIComponent(merchant) : "");
  };

  useEffect(() => {
    const update = () => {
      const path = window.location.hash.slice(2).split("?")[0];
      const selected = path.split("/")[0];
      setView(
        navigation.some(
          (n) => n.id === selected && (!n.admin || user?.is_admin),
        )
          ? selected
          : "overview",
      );
      const raw = new URLSearchParams(
        window.location.hash.split("?")[1] || "",
      ).get("merchant_id");
      if (user) setMerchant(raw && user.is_admin ? raw : String(user.id));
    };
    update();
    window.addEventListener("hashchange", update);
    return () => window.removeEventListener("hashchange", update);
  }, [user]);

  if (starting)
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Loading label="正在连接工作空间" />
      </div>
    );

  if (!user)
    return (
      <Login onLogin={loadMe} theme={theme} onToggleTheme={cycleMode} />
    );

  const selected = merchants.find((m) => String(m.id) === merchant) || user;
  const item = navigation.find((n) => n.id === view) || navigation[0];

  const props: PageProps = {
    navigate,
    baseURL,
    merchant,
    refresh,
    user,
    reload,
    notify,
    run,
    busy,
  };

  const pages: Record<string, React.ReactNode> = {
    overview: <Overview {...props} />,
    orders: <Orders {...props} />,
    events: <Events {...props} />,
    pools: <Pools {...props} />,
    device: <Devices {...props} />,
    settings: <SettingsPage {...props} />,
    apps: <Applications {...props} />,
    merchants: <Merchants {...props} />,
    subscriptions: <Subscriptions {...props} />,
    smtp: <SMTPPage {...props} />,
  };

  return (
    <div className="app-shell">
      {mobile && (
        <button
          className="mobile-shade"
          aria-label="关闭导航"
          onClick={() => setMobile(false)}
        />
      )}
      <aside className={"sidebar " + (mobile ? "is-open" : "")}>
        <a href="/admin" className="brand">
          <span className="brand-mark">
            <img src="/assets/ane.svg" alt="" />
          </span>
          <span>ANE</span>
          <span className="version-tag">v2.4</span>
        </a>

        <div className="workspace-switch">
          <Avatar size="sm" color="accent" variant="soft">
            <Avatar.Fallback className="font-semibold">
              {selected.name.slice(0, 1).toUpperCase()}
            </Avatar.Fallback>
          </Avatar>
          <div className="min-w-0 flex-1">
            <div className="truncate text-[13px] font-semibold leading-tight">
              {selected.name}
            </div>
            <div className="mt-0.5 text-[11px] text-muted tabular-nums">
              商户 #{selected.id}
            </div>
          </div>
          <ShieldCheck size={15} className="shrink-0 text-accent" />
        </div>

        <nav className="sidebar-nav" aria-label="主导航">
          {navigation
            .filter((n) => !n.admin || user.is_admin)
            .map((n) => (
              <div key={n.id}>
                {n.section && <p className="nav-label">{n.section}</p>}
                <Button
                  className={"nav-button " + (view === n.id ? "selected" : "")}
                  variant="ghost"
                  onPress={() => {
                    navigate(n.id);
                    setError("");
                    setMobile(false);
                  }}
                >
                  <n.icon size={17} strokeWidth={view === n.id ? 2.1 : 1.75} />
                  <span>{n.name}</span>
                  {view === n.id && <span className="active-dot" />}
                </Button>
              </div>
            ))}
        </nav>

        <div className="sidebar-footer">
          <div className="sidebar-user-card">
            <Avatar size="sm" color="default" variant="soft">
              <Avatar.Fallback className="font-semibold">
                {user.username.slice(0, 1).toUpperCase()}
              </Avatar.Fallback>
            </Avatar>
            <div className="min-w-0 flex-1">
              <p className="truncate text-xs font-semibold">{user.username}</p>
              <p className="text-[11px] text-muted">
                {user.is_admin ? "系统管理员" : "商户账户"}
              </p>
            </div>
            <Tooltip delay={350}>
              <Button
                isIconOnly
                size="sm"
                variant="ghost"
                aria-label="退出登录"
                onPress={() =>
                  run(async () => {
                    await api("logout", "POST", {});
                    setUser(null);
                  }, "已退出登录")
                }
              >
                <LogOut size={15} />
              </Button>
              <Tooltip.Content placement="top">退出登录</Tooltip.Content>
            </Tooltip>
          </div>
          <p className="mt-3 px-2 text-[10px] tracking-[0.14em] text-muted">
            ANE · 收款管理工作空间
          </p>
        </div>
      </aside>

      <div className="main-shell">
        <header className="topbar">
          <div className="flex items-center gap-3">
            <Button
              isIconOnly
              size="sm"
              variant="ghost"
              className="mobile-menu"
              aria-label="打开导航"
              onPress={() => setMobile(true)}
            >
              <Menu size={18} />
            </Button>
            <Breadcrumbs>
              <Breadcrumbs.Item onPress={() => navigate("overview")}>
                工作空间
              </Breadcrumbs.Item>
              <Breadcrumbs.Item>{item.name}</Breadcrumbs.Item>
            </Breadcrumbs>
          </div>

          <div className="flex items-center gap-2">
            <Chip
              size="sm"
              variant="soft"
              color={
                connected === null
                  ? "default"
                  : connected
                    ? "success"
                    : "danger"
              }
              className="gap-1.5 font-medium"
            >
              <span
                className={"status-dot " + (connected ? "is-live" : "")}
              />
              {connected === null
                ? "检查连接中"
                : connected
                  ? "系统已连接"
                  : "连接中断"}
            </Chip>

            <ThemeToggleButton resolved={theme} onToggle={cycleMode} />

            <Tooltip delay={350}>
              <Button
                isIconOnly
                size="sm"
                variant="ghost"
                aria-label="刷新数据"
                onPress={reload}
              >
                <RefreshCw
                  size={16}
                  className={spinning ? "spin-icon" : ""}
                />
              </Button>
              <Tooltip.Content placement="bottom">刷新数据</Tooltip.Content>
            </Tooltip>
          </div>
        </header>

        <main className="main-content">
          <div className="page-heading">
            <div>
              <p className="eyebrow">ANE / {view.toUpperCase()}</p>
              <h1>{item.name}</h1>
              <p>{descriptions[view]}</p>
            </div>
            {user.is_admin && !["merchants", "smtp"].includes(view) && (
              <Choice
                label="当前商户"
                value={merchant}
                onChange={(v) => {
                  setMerchant(v);
                  window.location.hash =
                    "/" + view + "?merchant_id=" + encodeURIComponent(v);
                  setError("");
                }}
                options={merchants.map((m) => ({
                  id: String(m.id),
                  label: m.name + " · #" + m.id,
                }))}
                className="merchant-select"
              />
            )}
          </div>

          {error && (
            <div className="mb-5">
              <Notice error>{error}</Notice>
            </div>
          )}

          <div key={view + ":" + merchant} className="page-stage">
            {pages[view]}
          </div>

          <footer className="page-footer">
            <span>ANE</span>
            <span>清晰连接每一笔收款 · v2.4</span>
          </footer>
        </main>
      </div>
    </div>
  );
}

function Login({
  onLogin,
  theme,
  onToggleTheme,
}: {
  onLogin: () => Promise<void>;
  theme: "light" | "dark";
  onToggleTheme: () => void;
}) {
  const [username, setUsername] = useState(""),
    [password, setPassword] = useState(""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");

  return (
    <div className="login-layout">
      <div className="login-side">
        <div className="flex items-center justify-between">
          <a className="brand" href="/admin">
            <span className="brand-mark">
              <img src="/assets/ane.svg" alt="" />
            </span>
            <span>ANE</span>
            <span className="version-tag">v2.4</span>
          </a>
          <ThemeToggleButton resolved={theme} onToggle={onToggleTheme} />
        </div>

        <div className="login-form">
          <p className="eyebrow">欢迎回来</p>
          <h1>
            连接收款，
            <br />
            专注你的业务。
          </h1>
          <p className="mt-4 text-sm leading-7 text-muted">
            使用你的管理员或商户账户登录 ANE 收款管理工作空间。
          </p>

          <form
            className="mt-8 space-y-5"
            onSubmit={async (e) => {
              e.preventDefault();
              setBusy(true);
              setError("");
              try {
                await api("/api/auth/login", "POST", { username, password });
                await onLogin();
              } catch (e) {
                setError((e as Error).message);
              } finally {
                setBusy(false);
              }
            }}
          >
            <TextField
              isRequired
              value={username}
              onChange={setUsername}
              className="w-full"
            >
              <Label>登录账号</Label>
              <Input autoComplete="username" placeholder="输入账号" />
            </TextField>

            <TextField
              isRequired
              type="password"
              value={password}
              onChange={setPassword}
              className="w-full"
            >
              <Label>密码</Label>
              <Input autoComplete="current-password" placeholder="输入密码" />
            </TextField>

            {error && <Notice error>{error}</Notice>}

            <Button
              type="submit"
              size="lg"
              fullWidth
              isPending={busy}
              className="mt-2 font-medium"
            >
              登录工作空间
              <ArrowRight size={17} />
            </Button>
          </form>

          <p className="mt-6 text-xs leading-6 text-muted">
            商户账号由管理员创建。如需重置密码，请联系系统管理员。
          </p>
        </div>

        <p className="text-xs text-muted">ANE · 收款管理工作空间</p>
      </div>

      <div className="login-visual">
        <div className="login-visual-grid" />
        <div className="orbit orbit-one" />
        <div className="orbit orbit-two" />
        <div className="orbit orbit-three" />

        <div className="login-visual-content">
          <div className="inline-flex items-center gap-2 rounded-full border border-white/15 bg-white/10 px-3 py-1 text-[11px] font-medium tracking-[0.2em] text-white/80 backdrop-blur-md">
            <Sparkles size={12} />
            ANE WORKSPACE
          </div>
          <h2>
            有序收款。
            <br />
            从容掌控。
          </h2>

          <Card className="visual-receipt">
            <Card.Content>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2 text-sm font-medium text-white">
                  <span className="status-dot is-live text-emerald-300" />
                  收款通道实时在线
                </div>
                <ShieldCheck size={20} className="text-emerald-200" />
              </div>

              <div className="my-6 h-px bg-white/20" />

              <div className="flex items-end justify-between gap-4">
                <div>
                  <div className="text-[11px] tracking-widest text-white/60">
                    UNIFIED LEDGER
                  </div>
                  <span className="mt-1 block text-4xl font-semibold tracking-tight text-white">
                    ANE
                  </span>
                </div>
                <span className="text-right text-xs leading-6 text-white/75">
                  独立商户 · 实时设备
                  <br />
                  智能优惠 · 可靠回调
                </span>
              </div>
            </Card.Content>
          </Card>

          <div className="login-feature-pills">
            <span className="login-feature-pill">
              <Radio size={12} />
              秒级设备心跳
            </span>
            <span className="login-feature-pill">
              <Layers size={12} />
              动态最小让利池
            </span>
            <span className="login-feature-pill">
              <ShieldCheck size={12} />
              多应用隔离签名
            </span>
          </div>

          <p className="mt-8 text-xs leading-7 text-white/60">
            把交易订单、收款设备与邮件提醒整合在同一工作空间，让日常收款管理清晰从容。
          </p>
        </div>
      </div>
    </div>
  );
}
