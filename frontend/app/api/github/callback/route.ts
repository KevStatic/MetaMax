import { NextRequest, NextResponse } from "next/server";

// GitHub OAuth callback — exchanges the temporary code for an access token,
// stores it in a short-lived cookie, and redirects back to /deploy.
export async function GET(req: NextRequest) {
  const { searchParams } = new URL(req.url);
  const code = searchParams.get("code");
  const state = searchParams.get("state");

  if (!code) {
    return NextResponse.redirect(new URL("/deploy?github_error=no_code", req.url));
  }

  const clientId = process.env.GITHUB_CLIENT_ID;
  const clientSecret = process.env.GITHUB_CLIENT_SECRET;

  if (!clientId || !clientSecret) {
    return NextResponse.redirect(new URL("/deploy?github_error=not_configured", req.url));
  }

  // Exchange code for access token
  const tokenRes = await fetch("https://github.com/login/oauth/access_token", {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      client_id: clientId,
      client_secret: clientSecret,
      code,
    }),
  });

  if (!tokenRes.ok) {
    return NextResponse.redirect(new URL("/deploy?github_error=token_exchange_failed", req.url));
  }

  const tokenData = await tokenRes.json();
  const accessToken = tokenData.access_token;

  if (!accessToken) {
    return NextResponse.redirect(new URL("/deploy?github_error=" + (tokenData.error ?? "unknown"), req.url));
  }

  // Store token in a secure httpOnly cookie (30 min expiry)
  const redirectUrl = state === "deploy" ? "/deploy" : "/deploy";
  const response = NextResponse.redirect(new URL(redirectUrl + "?github_connected=1", req.url));
  response.cookies.set("gh_token", accessToken, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    maxAge: 60 * 30, // 30 minutes
    path: "/",
  });

  return response;
}
