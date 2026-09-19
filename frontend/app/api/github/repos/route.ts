import { NextRequest, NextResponse } from "next/server";
import { cookies } from "next/headers";

// Returns the authenticated user's GitHub repos (up to 100, sorted by push date).
// Requires a valid gh_token cookie set by /api/github/callback.
export async function GET(req: NextRequest) {
  const cookieStore = await cookies();
  const token = cookieStore.get("gh_token")?.value;

  if (!token) {
    return NextResponse.json({ error: "not_connected" }, { status: 401 });
  }

  const { searchParams } = new URL(req.url);
  const query = searchParams.get("q") ?? "";
  const page = searchParams.get("page") ?? "1";

  let apiURL: string;
  if (query) {
    // Use search API for filtered queries
    apiURL = `https://api.github.com/search/repositories?q=${encodeURIComponent(query + " user:@me")}&sort=updated&per_page=30&page=${page}`;
  } else {
    // List all user repos sorted by latest push
    apiURL = `https://api.github.com/user/repos?sort=pushed&per_page=50&page=${page}&affiliation=owner,collaborator`;
  }

  const ghRes = await fetch(apiURL, {
    headers: {
      Authorization: `Bearer ${token}`,
      Accept: "application/vnd.github+json",
      "X-GitHub-Api-Version": "2022-11-28",
    },
    next: { revalidate: 0 },
  });

  if (ghRes.status === 401) {
    // Token expired — clear cookie
    const response = NextResponse.json({ error: "token_expired" }, { status: 401 });
    response.cookies.delete("gh_token");
    return response;
  }

  if (!ghRes.ok) {
    return NextResponse.json({ error: "github_api_error", status: ghRes.status }, { status: 502 });
  }

  const data = await ghRes.json();
  // Normalise both /user/repos and /search/repositories responses
  const repos: { full_name: string; html_url: string; description: string | null; private: boolean; pushed_at: string }[] =
    Array.isArray(data) ? data : (data.items ?? []);

  return NextResponse.json(
    repos.map((r) => ({
      full_name: r.full_name,
      html_url: r.html_url,
      description: r.description ?? "",
      private: r.private,
      pushed_at: r.pushed_at,
    }))
  );
}

// DELETE — disconnect GitHub (clear cookie)
export async function DELETE() {
  const response = NextResponse.json({ ok: true });
  response.cookies.delete("gh_token");
  return response;
}
