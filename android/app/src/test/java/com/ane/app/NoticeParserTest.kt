package com.ane.app
import org.junit.Assert.*
import org.junit.Test
class NoticeParserTest {
    @Test fun parsesPersonalAndMerchantReceiptsWithoutPayer() {
        val cases = listOf(
            Triple(NoticeParser.ALIPAY, "张**通过扫码向你付款0.01元", 1L),
            Triple(NoticeParser.ALIPAY, "你已成功收款0.01元.\n已转入余额 做生意收钱提现免费>>", 1L),
            Triple(NoticeParser.ALIPAY, "支付宝到账 12.3 元！", 1230L),
            Triple(NoticeParser.ALIPAY, "收款成功：￥999999.99元（商家收款）", 99999999L),
            Triple(NoticeParser.WECHAT, "[4条]微信收款助手: 微信支付收款0.01元(朋友到店)", 1L),
            Triple(NoticeParser.WECHAT, "微信支付收款0.01元(老顾客第3次消费)", 1L),
            Triple(NoticeParser.WECHAT, "微信收款：12.30元", 1230L)
        )
        cases.forEach { (pkg,text,cents) -> assertEquals(text, cents, NoticeParser.parse(pkg,text)!!.cents) }
    }
    @Test fun rejectsAmbiguousFailedAndInvalidReceipts() {
        listOf("成功收款0.00元", "成功收款1.001元", "成功收款01.01元", "成功收款1000000元", "退款到账1元", "收款失败1元", "待付款1元", "成功收款1元，余额2元", "微信支付收款1元").forEach { assertNull(it, NoticeParser.parse(NoticeParser.ALIPAY,it)) }
        assertNull(NoticeParser.parse(NoticeParser.WECHAT,"微信支付收款1元；微信支付收款1元"))
        assertNull(NoticeParser.parse("com.other.app","微信支付收款1元"))
    }
}
