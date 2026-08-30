export type StorefrontApiConfig = {
  baseUrl: string;
  host: string;
};

export function createStorefrontApiClient(config: StorefrontApiConfig) {
  return {
    async get(path: string): Promise<Response> {
      return fetch(new URL(path, config.baseUrl), {
        headers: { 'X-Storefront-Host': config.host }
      });
    }
  };
}
