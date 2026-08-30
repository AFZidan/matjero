import React from 'react';
import { createRoot } from 'react-dom/client';
import { createApiClient } from './lib/api';
import { directionFor, messages, type Locale } from './i18n/locales';
import './styles.css';

type Bootstrap = {
  actor: string;
  direction: 'rtl' | 'ltr';
  principal?: { subject: string; preferred_username?: string };
  markets: Array<{ code: string; country: { name: string }; currency: { code: string } }>;
};

type Seller = { id: string; code: string; name: string; status: string };
type Store = { id: string; seller_id: string; market_code: string; code: string; name: string; status: string };
type Offer = { offer_id?: string; id?: string; market_code: string; supplier_name?: string; supplier_code?: string; product_name?: string; status: string; price?: { amount_minor: number; currency: string } | null };
type Listing = { id: string; store_id: string; product_id: string; market_code: string; status: string };

const locale = (new URLSearchParams(window.location.search).get('locale') === 'ar' ? 'ar' : 'en') satisfies Locale;
const copy = messages[locale];
const api = createApiClient({ baseUrl: import.meta.env.VITE_API_BASE_URL ?? window.location.origin });
document.documentElement.lang = locale;
document.documentElement.dir = directionFor(locale);

function App() {
  const [bootstrap, setBootstrap] = React.useState<Bootstrap | null>(null);
  const [seller, setSeller] = React.useState<Seller | null>(null);
  const [stores, setStores] = React.useState<Store[]>([]);
  const [offers, setOffers] = React.useState<Offer[]>([]);
  const [listings, setListings] = React.useState<Listing[]>([]);
  const [error, setError] = React.useState<string | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [profileName, setProfileName] = React.useState('');
  const [profileStatus, setProfileStatus] = React.useState('active');
  const [profileSettings, setProfileSettings] = React.useState('{"channel":"retail"}');
  const [storeForm, setStoreForm] = React.useState({ market_code: 'EG', code: '', name: '', status: 'active' });
  const [catalogForm, setCatalogForm] = React.useState({ store_id: '', market_code: 'EG', supplier_id: '', category_id: '', search: '' });
  const [listingForm, setListingForm] = React.useState({ store_id: '', product_id: '', supplier_offer_id: '', market_code: 'EG', status: 'draft' });
  const [priceForm, setPriceForm] = React.useState({ amount_minor: 0, currency: 'EGP' });

  React.useEffect(() => {
    let active = true;
    async function load() {
      try {
        setLoading(true);
        const [bootRes, profileRes, storesRes] = await Promise.all([
          api.get(`/v1/bootstrap?locale=${locale}`),
          api.get(`/v1/seller/profile?locale=${locale}`),
          api.get(`/v1/seller/stores?locale=${locale}`)
        ]);
        if (!active) return;
        setBootstrap(await bootRes.json());
        const profile = await profileRes.json() as { seller: Seller };
        setSeller(profile.seller);
        setProfileName(profile.seller.name);
        setProfileStatus(profile.seller.status);
        const storeItems = (await storesRes.json() as { items: Store[] }).items;
        setStores(storeItems);
        if (storeItems[0]) {
          setCatalogForm((current) => ({ ...current, store_id: storeItems[0].id }));
          const offersResponse = await api.get(`/v1/seller/listings?store_id=${encodeURIComponent(storeItems[0].id)}&locale=${locale}`);
          setListings((await offersResponse.json() as { items: Listing[] }).items);
        }
      } catch (err) {
        if (active) setError(err instanceof Error ? err.message : 'Failed to load seller dashboard');
      } finally {
        if (active) setLoading(false);
      }
    }
    void load();
    return () => {
      active = false;
    };
  }, []);

  async function submitProfile() {
    const response = await api.put(`/v1/seller/profile?locale=${locale}`, {
      name: profileName,
      status: profileStatus,
      settings: JSON.parse(profileSettings || '{}')
    });
    if (!response.ok) throw new Error('Profile update failed');
  }

  async function submitStore() {
    const response = await api.post(`/v1/seller/stores?locale=${locale}`, storeForm);
    if (!response.ok) throw new Error('Store create failed');
  }

  async function loadOffers() {
    const query = new URLSearchParams({
      store_id: catalogForm.store_id,
      supplier_id: catalogForm.supplier_id,
      category_id: catalogForm.category_id,
      search: catalogForm.search,
      locale
    });
    const response = await api.get(`/v1/seller/catalog/offers?${query.toString()}`);
    if (!response.ok) throw new Error('Offer discovery failed');
    const data = await response.json() as { items: Offer[] };
    setOffers(data.items);
  }

  async function importListing() {
    const response = await api.post(`/v1/seller/listings/import?locale=${locale}`, listingForm);
    if (!response.ok) throw new Error('Listing import failed');
  }

  async function updatePrice(listingId: string) {
    const response = await api.post(`/v1/seller/listings/${listingId}/price?locale=${locale}`, priceForm);
    if (!response.ok) throw new Error('Listing price update failed');
  }

  return (
    <main className="app-shell">
      <header className="hero">
        <div>
          <p className="eyebrow">{bootstrap?.actor ?? 'seller'}</p>
          <h1>{copy.appName}</h1>
          <p className="lede">Create stores, browse supplier offers in-market, and manage seller listings.</p>
        </div>
        <div className="hero-meta">
          <span className="pill">{bootstrap?.direction ?? directionFor(locale)}</span>
          <span className="pill">{bootstrap?.principal?.preferred_username ?? bootstrap?.principal?.subject ?? 'anonymous'}</span>
        </div>
      </header>

      {error ? <div className="notice notice-error">{error}</div> : null}
      {loading ? <div className="notice">{copy.status}</div> : null}

      <section className="panel-grid">
        <Panel title="Seller Profile">
          <FormGrid>
            <input value={profileName} onChange={(e) => setProfileName(e.target.value)} placeholder="seller name" />
            <input value={profileStatus} onChange={(e) => setProfileStatus(e.target.value)} placeholder="status" />
            <textarea value={profileSettings} onChange={(e) => setProfileSettings(e.target.value)} rows={3} placeholder="JSON settings" />
            <button onClick={() => void submitProfile()}>Save profile</button>
          </FormGrid>
          {seller ? <div className="hint">{seller.code}</div> : null}
        </Panel>
        <Panel title="Stores">
          <Stack>{stores.map((store) => <Row key={store.id} title={store.name} meta={`${store.code} · ${store.market_code}`} status={store.status} />)}</Stack>
          <FormGrid>
            <input value={storeForm.market_code} onChange={(e) => setStoreForm({ ...storeForm, market_code: e.target.value })} placeholder="market code" />
            <input value={storeForm.code} onChange={(e) => setStoreForm({ ...storeForm, code: e.target.value })} placeholder="code" />
            <input value={storeForm.name} onChange={(e) => setStoreForm({ ...storeForm, name: e.target.value })} placeholder="name" />
            <input value={storeForm.status} onChange={(e) => setStoreForm({ ...storeForm, status: e.target.value })} placeholder="status" />
            <button onClick={() => void submitStore()}>Create store</button>
          </FormGrid>
        </Panel>
      </section>

      <section className="panel-grid">
        <Panel title="Supplier Catalog Discovery">
          <FormGrid>
            <input value={catalogForm.store_id} onChange={(e) => setCatalogForm({ ...catalogForm, store_id: e.target.value })} placeholder="store id" />
            <input value={catalogForm.supplier_id} onChange={(e) => setCatalogForm({ ...catalogForm, supplier_id: e.target.value })} placeholder="supplier id" />
            <input value={catalogForm.category_id} onChange={(e) => setCatalogForm({ ...catalogForm, category_id: e.target.value })} placeholder="category id" />
            <input value={catalogForm.search} onChange={(e) => setCatalogForm({ ...catalogForm, search: e.target.value })} placeholder="search" />
            <button onClick={() => void loadOffers()}>Browse offers</button>
          </FormGrid>
          <Stack>{offers.map((offer) => <Row key={offer.id ?? offer.offer_id ?? `${offer.market_code}-${offer.product_name}`} title={offer.product_name ?? 'Offer'} meta={`${offer.supplier_name ?? offer.supplier_code ?? 'supplier'} · ${offer.market_code}`} status={offer.status} />)}</Stack>
        </Panel>
      </section>

      <section className="panel-grid">
        <Panel title="Listings">
          <Stack>{listings.map((listing) => <Row key={listing.id} title={listing.id} meta={`${listing.store_id} · ${listing.market_code}`} status={listing.status} />)}</Stack>
          <FormGrid>
            <input value={listingForm.store_id} onChange={(e) => setListingForm({ ...listingForm, store_id: e.target.value })} placeholder="store id" />
            <input value={listingForm.product_id} onChange={(e) => setListingForm({ ...listingForm, product_id: e.target.value })} placeholder="product id" />
            <input value={listingForm.supplier_offer_id} onChange={(e) => setListingForm({ ...listingForm, supplier_offer_id: e.target.value })} placeholder="supplier offer id" />
            <input value={listingForm.market_code} onChange={(e) => setListingForm({ ...listingForm, market_code: e.target.value })} placeholder="market code" />
            <input value={listingForm.status} onChange={(e) => setListingForm({ ...listingForm, status: e.target.value })} placeholder="status" />
            <button onClick={() => void importListing()}>Import listing</button>
          </FormGrid>
          <FormGrid>
            <input type="number" value={priceForm.amount_minor} onChange={(e) => setPriceForm({ ...priceForm, amount_minor: Number(e.target.value) })} placeholder="amount minor" />
            <input value={priceForm.currency} onChange={(e) => setPriceForm({ ...priceForm, currency: e.target.value })} placeholder="currency" />
          </FormGrid>
          {listings[0] ? <button onClick={() => void updatePrice(listings[0].id)}>Update first listing price</button> : null}
        </Panel>
      </section>
    </main>
  );
}

function Panel({ title, children }: { title: string; children: React.ReactNode }) {
  return <section className="panel"><div className="panel-head"><div><h2>{title}</h2></div></div>{children}</section>;
}

function Stack({ children }: { children: React.ReactNode }) {
  return <div className="stack">{children}</div>;
}

function Row({ title, meta, status }: { title: string; meta: string; status: string }) {
  return <article className="row-card"><div className="row-copy"><strong>{title}</strong><span>{meta}</span></div><span className={`badge badge-${status.toLowerCase().replace(/[^a-z0-9]+/g, '-')}`}>{status}</span></article>;
}

function FormGrid({ children }: { children: React.ReactNode }) {
  return <div className="form-grid">{children}</div>;
}

createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
