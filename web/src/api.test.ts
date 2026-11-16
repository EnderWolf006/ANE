import { afterEach, describe, expect, it, vi } from "vitest";
import { api, ApiError, scoped, maskAppKey } from "./api";
afterEach(() => vi.unstubAllGlobals());
describe("API session and failure handling", () => {
  it("returns to login when a management session expires", async () => {
    const events = new EventTarget();
    const logout = vi.fn();
    events.addEventListener("ane:logout", logout);
    vi.stubGlobal("window", events);
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          new Response(JSON.stringify({ msg: "请重新登录" }), { status: 401 }),
        ),
    );
    await expect(api("orders")).rejects.toMatchObject({
      status: 401,
      message: "请重新登录",
    });
    expect(logout).toHaveBeenCalledOnce();
  });
  it("keeps login errors in the login form", async () => {
    const dispatchEvent = vi.fn();
    vi.stubGlobal("window", { dispatchEvent });
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ msg: "账号或密码错误" }), {
          status: 401,
        }),
      ),
    );
    await expect(api("/api/auth/login", "POST", {})).rejects.toBeInstanceOf(
      ApiError,
    );
    expect(dispatchEvent).not.toHaveBeenCalled();
  });
  it("handles a reverse proxy HTML error without exposing HTML to the UI", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          new Response("<html>Bad Gateway</html>", { status: 502 }),
        ),
    );
    await expect(api("orders")).rejects.toMatchObject({
      status: 502,
      message: "服务端响应无效，请稍后重试",
    });
  });
  it("preserves pagination and safely encodes the merchant scope", () => {
    expect(scoped("orders?page=2", "1001&merchant_id=1000")).toBe(
      "orders?page=2&merchant_id=1001%26merchant_id%3D1000",
    );
  });
  it("masks the random key and preserves the complete application suffix", () => {
    expect(maskAppKey("0123456789abcdef_my_shop_2", "my_shop_2")).toBe(
      "0123******cdef_my_shop_2",
    );
  });
});
