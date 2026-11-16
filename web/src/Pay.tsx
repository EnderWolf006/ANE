import { useEffect, useState } from "react";
import { Card, Chip } from "@heroui/react";
import { CheckCircle2, Clock3, ShieldCheck } from "lucide-react";
import { PaymentChannel } from "./PaymentChannel";
import { api, money } from "./api";
import { Loading, Notice } from "./components";
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
  const trade = location.pathname.split("/")[2];
  const [data, setData] = useState<Payment>(),
    [error, setError] = useState(""),
    [now, setNow] = useState(Date.now()),
    [offset, setOffset] = useState(0);
  useEffect(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      try {
        const d = await api<Payment>("/api/pay/" + encodeURIComponent(trade));
        if (!active) return;
        setData(d);
        setOffset(Date.parse(d.server_time) - Date.now());
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
  return (
    <div className="pay-layout">
      <a href="/" className="brand">
        <img src="/assets/ane.svg" alt="" />
        ANE
      </a>
      <Card className="pay-card">
        <Card.Content>
          {!data ? (
            <>{error ? <Notice error>{error}</Notice> : <Loading />}</>
          ) : (
            <>
              <div className="text-center">
                <Chip
                  size="sm"
                  variant="soft"
                  color={data.status === "paid" ? "success" : "accent"}
                >
                  <PaymentChannel type={data.payment_type} label={data.status === "paid" ? channel + "付款已确认" : channel + "付款"} />
                </Chip>
                <h1 className="mt-2 text-lg font-semibold">{data.name}</h1>
                <p className="mt-1 text-xs text-muted">
                  {data.status === "paid"
                    ? "感谢你的付款，收款信息已确认。"
                    : "打开" + channel + "扫一扫，输入下方精确金额。"}
                </p>
                <div className="payment-amount">
                  {money(data.pay_amount_cents)}
                </div>
                {data.discount_cents > 0 && (
                  <div className="mt-1 flex flex-wrap items-center justify-center gap-3 text-xs">
                    <span className="text-muted">原价 <s>{money(data.amount_cents)}</s></span>
                    <span className="font-medium text-success">已优惠 {money(data.discount_cents)}</span>
                  </div>
                )}
              </div>
              {data.status === "paid" ? (
                <div className="payment-success">
                  <CheckCircle2 size={52} strokeWidth={1.2} />
                  <h2>支付成功</h2>
                  {data.return_url && (
                    <a href={data.return_url} className="primary-link">
                      返回商户
                    </a>
                  )}
                </div>
              ) : data.status === "expired" || seconds === 0 ? (
                <div className="payment-expired">
                  <Clock3 size={42} />
                  <h2>订单已超时</h2>
                  <p>请返回业务网站重新下单，请勿继续付款。</p>
                </div>
              ) : (
                <>
                  <img
                    className="payment-qr"
                    src={"/api/pay/" + encodeURIComponent(trade) + "/qr"}
                    alt={channel + "收款码"}
                  />
                  <div className="payment-countdown">
                    <Clock3 size={14} />
                    剩余付款时间 {Math.floor(seconds / 60)}:
                    {String(seconds % 60).padStart(2, "0")}
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
                <div className="mt-2">
                  <Notice error>
                    连接暂时中断，正在重试。已付款请勿重复支付。
                  </Notice>
                </div>
              )}
              <div className="mt-3 border-t border-border pt-2 text-center">
                <p className="text-[10px] text-muted">
                  商户订单 {data.out_trade_no}
                </p>
                <p className="mt-1 text-[10px] text-muted">
                  <ShieldCheck size={12} className="mr-1 inline" />由 ANE
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
