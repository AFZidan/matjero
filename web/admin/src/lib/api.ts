export type ApiConfig = {
  baseUrl: string;
  getAccessToken?: () => Promise<string | null>;
};

export function createApiClient(config: ApiConfig) {
  async function request(method: string, path: string, body?: unknown): Promise<Response> {
    const token = await config.getAccessToken?.();
    return fetch(new URL(path, config.baseUrl), {
      method,
      headers: {
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {})
      },
      body: body === undefined ? undefined : JSON.stringify(body)
    });
  }

  return {
    async get(path: string): Promise<Response> {
      const token = await config.getAccessToken?.();
      return fetch(new URL(path, config.baseUrl), {
        headers: token ? { Authorization: `Bearer ${token}` } : undefined
      });
    },
    post(path: string, body?: unknown): Promise<Response> {
      return request('POST', path, body);
    },
    put(path: string, body?: unknown): Promise<Response> {
      return request('PUT', path, body);
    }
  };
}
