#!/usr/bin/env python3
"""A minimal EPay merchant client. No financial transaction is performed by this script."""
import argparse
import hashlib
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request

p = argparse.ArgumentParser(description='创建 ANE 易支付测试订单，返回收银台 URL')
p.add_argument('--base', required=True)
p.add_argument('--pid', default='1000')
p.add_argument('--type', choices=['alipay','wxpay'], default='alipay')
p.add_argument('--out-trade-no', required=True)
p.add_argument('--money', required=True)
p.add_argument('--notify', required=True)
p.add_argument('--return-url', default='')
p.add_argument('--name', default='ANE 测试商品')
a = p.parse_args()
key = os.environ.get('ANE_MERCHANT_KEY')
if not key:
    sys.exit('请通过 ANE_MERCHANT_KEY 环境变量设置商户密钥')
values = dict(pid=a.pid, type=a.type, out_trade_no=a.out_trade_no, money=a.money,
              notify_url=a.notify, return_url=a.return_url, name=a.name)
canonical = '&'.join(f'{k}={values[k]}' for k in sorted(values) if values[k])
values['sign'] = hashlib.md5((canonical + key).encode()).hexdigest()
values['sign_type'] = 'MD5'
req = urllib.request.Request(a.base.rstrip('/') + '/mapi.php', data=urllib.parse.urlencode(values).encode())
try:
    with urllib.request.urlopen(req, timeout=20) as response:
        print(json.dumps(json.load(response), ensure_ascii=False, indent=2))
except urllib.error.HTTPError as e:
    print(e.read().decode(), file=sys.stderr)
    sys.exit(1)
