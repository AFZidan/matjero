export type ApiConfig = {
  baseUrl: string;
  getAccessToken?: () => Promise<string | null>;
};

export function createApiClient(config: ApiConfig) {
  return {
    async get(path: string): Promise<Response> {
      const token = await config.getAccessToken?.();
      return fetch(new URL(path, config.baseUrl), {
        headers: token ? { Authorization: `Bearer ${token}` } : undefined
      });
    }
  };
}
