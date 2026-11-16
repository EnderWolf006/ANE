package com.ane.app
import org.junit.Assert.*
import org.junit.Test
class NoticeParserTest {
    @Test fun parsesExactCentsAndPayer() {
        val payment = NoticeParser.parse("张**通过扫码向你付款0.01元")!!
        assertEquals("张**", payment.payer); assertEquals(1L, payment.cents)
        assertEquals(1230L, NoticeParser.parse(" 李四通过扫码向你付款12.3元。 ")!!.cents)
    }
    @Test fun rejectsOtherMessagesAndInvalidAmounts() {
        listOf("支付宝到账0.01元", "张三通过扫码向你付款0.00元", "张三通过扫码向你付款1.001元", "张三通过扫码向你付款01.01元", "通过扫码向你付款1元", "张三通过扫码向你付款9999999元", "张三通过扫码向你付款1元 另一条").forEach { assertNull(it, NoticeParser.parse(it)) }
    }
}
