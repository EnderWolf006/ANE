type PaymentType = "alipay" | "wxpay";

export function PaymentChannel({ type, label }: { type: PaymentType; label?: string }) {
  const wechat = type === "wxpay";
  return (
    <span className="inline-flex items-center gap-1.5 align-middle">
      <img
        src={"/assets/" + (wechat ? "wechat" : "alipay") + ".svg"}
        className="h-4 w-4 shrink-0"
        width={16}
        height={16}
        alt=""
        aria-hidden="true"
      />
      <span>{label ?? (wechat ? "微信支付" : "支付宝")}</span>
    </span>
  );
}
