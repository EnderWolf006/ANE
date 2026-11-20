import { useEffect, useRef, useState } from "react";
import { Button, Card, Chip, ProgressBar, Separator, toast } from "@heroui/react";
import {
  ArrowRight,
  Check,
  CheckCircle2,
  Clock3,
  Copy,
  Moon,
  ShieldCheck,
  Sun,
} from "lucide-react";
import { PaymentChannel } from "./PaymentChannel";
import { api, money } from "./api";
import { Loading, Notice } from "./components";
import { useThemeMode } from "./theme";

interface Payment {
  payment_type: "alipay" | "wxpay";
  payment_note: string;
  trade_no: string;
  out_trade_no: string;
  name: string;
  amount_cents: number;
  pay_amount_cents: number;
  discount_cents: number;
  status: string;
  expires_at: string;
  server_time: string;
  return_url: string;
}

export function Pay() {
  const { resolved: theme, cycleMode } = useThemeMode();
  const trade = location.pathname.split("/")[2];
  const [data, setData] = useState<Payment>(),
    [error, setError] = useState(""),
    [now, setNow] = useState(Date.now()),
    [offset, setOffset] = useState(0),
    [copied, setCopied] = useState(false);
  const initialTotalRef = useRef<number>(0);

  useEffect(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      try {
        const d = await api<Payment>("/api/pay/" + encodeURIComponent(trade));
        if (!active) return;
        setData(d);
        const serverOffset = Date.parse(d.server_time) - Date.now();
        setOffset(serverOffset);
        if (!initialTotalRef.current) {
          const remain = Math.max(
            1,
            Math.ceil((Date.parse(d.expires_at) - Date.parse(d.server_time)) / 1000),
          );
          initialTotalRef.current = remain;
        }
        setError("");
        if (d.status === "paid") return;
      } catch (e) {
        if (active) setError((e as Error).message);
      }
      if (active) timer = setTimeout(poll, 3000);
    }
    poll();
    const clock = setInterval(() => setNow(Date.now()), 1000);
    return () => {
      active = false;
      clearTimeout(timer);
      clearInterval(clock);
    };
  }, [trade]);

  const channel = data?.payment_type === "wxpay" ? "微信" : "支付宝";
  const seconds = data
    ? Math.max(
        0,
        Math.ceil((Date.parse(data.expires_at) - now - offset) / 1000),
      )
    : 0;
  const totalSeconds = Math.max(initialTotalRef.current || seconds || 1, 1);
  const progress = Math.min(100, Math.max(0, (seconds / totalSeconds) * 100));
  const urgent = seconds > 0 && seconds <= 60;

  const copyAmount = async () => {
    if (!data) return;
    const text = (data.pay_amount_cents / 100).toFixed(2);
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      toast.success("已复制精确付款金额 " + text);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.danger("复制失败，请手动输入金额");
    }
  };

  return (
    <div className="pay-layout">
      <div className="pay-topbar">
        <a href="/" className="brand">
          <span className="brand-mark">
            <img src="/assets/ane.svg" alt="" />
          </span>
          <span>ANE</span>
        </a>
        <Button
          isIconOnly
          size="sm"
          variant="ghost"
          aria-label={theme === "dark" ? "切换到浅色模式" : "切换到深色模式"}
          onPress={cycleMode}
        >
          {theme === "dark" ? <Sun size={16} /> : <Moon size={16} />}
        </Button>
      </div>

      <Card className="pay-card">
        <Card.Content>
          {!data ? (
            <>{error ? <Notice error>{error}</Notice> : <Loading label="正在获取订单信息" />}</>
          ) : (
            <>
              <div className="text-center">
                <Chip
                  size="sm"
                  variant="soft"
                  color={data.status === "paid" ? "success" : "accent"}
                  className="font-medium"
                >
                  <PaymentChannel
                    type={data.payment_type}
                    label={
                      data.status === "paid"
                        ? channel + "付款已确认"
                        : channel + "扫码付款"
                    }
                  />
                </Chip>

                <h1 className="mt-3 text-lg font-semibold tracking-tight text-foreground">
                  {data.name}
                </h1>
                <p className="mt-1 text-xs leading-5 text-muted">
                  {data.status === "paid"
                    ? "感谢你的付款，收款信息已实时确认。"
                    : "打开" + channel + "扫一扫，并输入下方精确金额完成支付。"}
                </p>

                <div className="payment-amount-box">
                  <div className="flex items-center justify-center gap-2">
                    <div className="payment-amount">
                      {money(data.pay_amount_cents)}
                    </div>
                    {data.status !== "paid" &&
                      data.status !== "expired" &&
                      seconds > 0 && (
                        <Button
                          isIconOnly
                          size="sm"
                          variant="secondary"
                          aria-label="复制精确付款金额"
                          onPress={copyAmount}
                        >
                          {copied ? <Check size={14} /> : <Copy size={14} />}
                        </Button>
                      )}
                  </div>
                  {data.discount_cents > 0 && (
                    <div className="mt-2 flex flex-wrap items-center justify-center gap-2 text-xs tabular-nums">
                      <span className="text-muted">
                        原价 <s>{money(data.amount_cents)}</s>
                      </span>
                      <Chip size="sm" variant="soft" color="success">
                        已立减 {money(data.discount_cents)}
                      </Chip>
                    </div>
                  )}
                </div>
              </div>

              {data.status === "paid" ? (
                <div className="payment-success">
                  <div className="grid size-16 place-items-center rounded-full bg-success-soft text-success">
                    <CheckCircle2 size={38} strokeWidth={1.6} />
                  </div>
                  <h2>支付成功</h2>
                  <p className="text-xs text-muted">
                    订单已完成确认，正在同步业务状态
                  </p>
                  {data.return_url && (
                    <a
                      href={data.return_url}
                      className="button button--primary mt-2 inline-flex items-center gap-1.5 px-6"
                    >
                      返回商户页面
                      <ArrowRight size={15} />
                    </a>
                  )}
                </div>
              ) : data.status === "expired" || seconds === 0 ? (
                <div className="payment-expired">
                  <div className="grid size-16 place-items-center rounded-full bg-surface-secondary text-muted">
                    <Clock3 size={34} strokeWidth={1.6} />
                  </div>
                  <h2>订单已超时</h2>
                  <p>请返回业务网站重新下单，请勿继续向该收款码付款。</p>
                </div>
              ) : (
                <>
                  <div className="payment-qr-stage">
                    <img
                      className="payment-qr"
                      src={"/api/pay/" + encodeURIComponent(trade) + "/qr"}
                      alt={channel + "收款码"}
                    />
                  </div>

                  <div className="space-y-2">
                    <div
                      className={
                        "payment-countdown " + (urgent ? "is-urgent" : "")
                      }
                    >
                      <span className="inline-flex items-center gap-1.5">
                        <Clock3 size={14} />
                        剩余付款时间
                      </span>
                      <strong className="font-semibold tabular-nums">
                        {Math.floor(seconds / 60)}:
                        {String(seconds % 60).padStart(2, "0")}
                      </strong>
                    </div>

                    <ProgressBar
                      aria-label="付款剩余时间"
                      value={progress}
                      size="sm"
                      color={urgent ? "warning" : "accent"}
                    >
                      <ProgressBar.Track>
                        <ProgressBar.Fill />
                      </ProgressBar.Track>
                    </ProgressBar>
                  </div>

                  {data.payment_note && (
                    <div
                      className="payment-note"
                      dangerouslySetInnerHTML={{ __html: data.payment_note }}
                    />
                  )}
                </>
              )}

              {error && (
                <div className="mt-3">
                  <Notice error>
                    连接暂时中断，正在重试。已付款请勿重复支付。
                  </Notice>
                </div>
              )}

              <Separator className="my-4" />

              <div className="space-y-1 text-center text-[11px] text-muted">
                <p className="font-mono">商户订单 {data.out_trade_no}</p>
                <p className="inline-flex items-center justify-center gap-1">
                  <ShieldCheck size={12} className="text-accent" />由 ANE
                  提供收款连接
                </p>
              </div>
            </>
          )}
        </Card.Content>
      </Card>
    </div>
  );
}
