import React from 'react';
import { createRoot } from 'react-dom/client';
import { createApiClient } from './lib/api';
import { directionFor, messages, type Locale } from './i18n/locales';
import './styles.css';

type Market = { code: string; country: { name: string }; currency: { code: string } };
type SupplierBootstrap = {
  actor: string;
  direction: 'rtl' | 'ltr';
  principal?: { subject: string; preferred_username?: string };
  markets: Market[];
};

type Supplier = { id: string; code: string; name: string; status: string };
type MarketRecord = { id: string; market_code: string; status: string };
type Location = { id: string; code: string; name: string; market_code: string; location_type: string; status: string };
type Product = { id: string; slug: string; status: string };
type Offer = { id: string; market_code: string; status: string; supplier_code?: string };
type Snapshot = { id: string; fulfillment_location_id: string; sku_id: string; on_hand_qty: number; reserved_qty: number; version: number };
type Movement = { id: string; movement_type: string; quantity_delta: number; on_hand_qty: number; reserved_qty: number; created_at: string };

const locale = (new URLSearchParams(window.location.search).get('locale') === 'ar' ? 'ar' : 'en') satisfies Locale;
const copy = messages[locale];
const api = createApiClient({ baseUrl: import.meta.env.VITE_API_BASE_URL ?? window.location.origin });
document.documentElement.lang = locale;
document.documentElement.dir = directionFor(locale);

function App() {
  const [bootstrap, setBootstrap] = React.useState<SupplierBootstrap | null>(null);
  const [supplier, setSupplier] = React.useState<Supplier | null>(null);
  const [markets, setMarkets] = React.useState<MarketRecord[]>([]);
  const [locations, setLocations] = React.useState<Location[]>([]);
  const [products, setProducts] = React.useState<Product[]>([]);
  const [offers, setOffers] = React.useState<Offer[]>([]);
  const [snapshots, setSnapshots] = React.useState<Snapshot[]>([]);
  const [movements, setMovements] = React.useState<Movement[]>([]);
  const [error, setError] = React.useState<string | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [profileName, setProfileName] = React.useState('');
  const [profileStatus, setProfileStatus] = React.useState('active');
  const [profileSettings, setProfileSettings] = React.useState('{"tone":"stable"}');
  const [locationForm, setLocationForm] = React.useState({ supplier_market_id: '', market_code: 'EG', code: '', name: '', location_type: 'warehouse', status: 'active' });
  const [productForm, setProductForm] = React.useState({ slug: '', status: 'active', supplier_code: '', ar_name: '', en_name: '' });
  const [offerForm, setOfferForm] = React.useState({ supplier_product_id: '', supplier_market_id: '', market_code: 'EG', status: 'active', amount_minor: 0, currency: 'EGP', is_available: true, available_qty: 0 });
  const [inventoryForm, setInventoryForm] = React.useState({ fulfillment_location_id: '', sku_id: '', on_hand_qty: 0, movement_type: 'adjust', quantity_delta: 1, reason: '' });
  const [selectedSnapshot, setSelectedSnapshot] = React.useState('');

  React.useEffect(() => {
    let active = true;
    async function load() {
      try {
        setLoading(true);
        const [bootRes, profileRes, marketsRes, locationsRes, productsRes, offersRes, inventoryRes] = await Promise.all([
          api.get(`/v1/bootstrap?locale=${locale}`),
          api.get(`/v1/supplier/profile?locale=${locale}`),
          api.get(`/v1/supplier/markets?locale=${locale}`),
          api.get(`/v1/supplier/locations?locale=${locale}`),
          api.get(`/v1/supplier/products?locale=${locale}`),
          api.get(`/v1/supplier/offers?locale=${locale}`),
          api.get(`/v1/supplier/inventory?locale=${locale}`)
        ]);
        if (!active) return;
        setBootstrap(await bootRes.json());
        const profile = await profileRes.json() as { supplier: Supplier };
        setSupplier(profile.supplier);
        setProfileName(profile.supplier.name);
        setProfileStatus(profile.supplier.status);
        setMarkets((await marketsRes.json() as { items: MarketRecord[] }).items);
        setLocations((await locationsRes.json() as { items: Location[] }).items);
        setProducts((await productsRes.json() as { items: Product[] }).items);
        setOffers((await offersRes.json() as { items: Offer[] }).items);
        setSnapshots((await inventoryRes.json() as { items: Snapshot[] }).items);
      } catch (err) {
        if (active) setError(err instanceof Error ? err.message : 'Failed to load supplier dashboard');
      } finally {
        if (active) setLoading(false);
      }
    }
    void load();
    return () => {
      active = false;
    };
  }, []);

  async function reloadInventory() {
    const inventoryRes = await api.get(`/v1/supplier/inventory?locale=${locale}`);
    setSnapshots((await inventoryRes.json() as { items: Snapshot[] }).items);
  }

  async function submitProfile() {
    const response = await api.put(`/v1/supplier/profile?locale=${locale}`, {
      name: profileName,
      status: profileStatus,
      settings: JSON.parse(profileSettings || '{}')
    });
    if (!response.ok) throw new Error('Profile update failed');
  }

  async function submitLocation() {
    const response = await api.post(`/v1/supplier/locations?locale=${locale}`, locationForm);
    if (!response.ok) throw new Error('Location create failed');
  }

  async function submitProduct() {
    const response = await api.post(`/v1/supplier/products?locale=${locale}`, {
      slug: productForm.slug,
      status: productForm.status,
      supplier_code: productForm.supplier_code,
      translations: [
        { locale: 'ar', name: productForm.ar_name, description: productForm.ar_name },
        { locale: 'en', name: productForm.en_name, description: productForm.en_name }
      ],
      category_ids: []
    });
    if (!response.ok) throw new Error('Product create failed');
  }

  async function submitOffer() {
    const response = await api.post(`/v1/supplier/offers?locale=${locale}`, {
      supplier_product_id: offerForm.supplier_product_id,
      supplier_market_id: offerForm.supplier_market_id,
      market_code: offerForm.market_code,
      status: offerForm.status,
      price: { amount_minor: offerForm.amount_minor, currency: offerForm.currency },
      is_available: offerForm.is_available,
      available_qty: offerForm.available_qty
    });
    if (!response.ok) throw new Error('Offer create failed');
  }

  async function submitInventory() {
    const response = await api.post(`/v1/supplier/inventory/snapshots?locale=${locale}`, inventoryForm);
    if (!response.ok) throw new Error('Inventory snapshot create failed');
    await reloadInventory();
  }

  async function adjustInventory(snapshotId: string) {
    const response = await api.post(`/v1/supplier/inventory/${snapshotId}/adjustments?locale=${locale}`, {
      quantity_delta: inventoryForm.quantity_delta,
      movement_type: inventoryForm.movement_type,
      reason: inventoryForm.reason
    });
    if (!response.ok) throw new Error('Inventory adjustment failed');
    await reloadInventory();
  }

  return (
    <main className="app-shell">
      <header className="hero">
        <div>
          <p className="eyebrow">{bootstrap?.actor ?? 'supplier'}</p>
          <h1>{copy.appName}</h1>
          <p className="lede">Manage supplier-owned commerce data, pricing, locations, and inventory.</p>
        </div>
        <div className="hero-meta">
          <span className="pill">{bootstrap?.direction ?? directionFor(locale)}</span>
          <span className="pill">{bootstrap?.principal?.preferred_username ?? bootstrap?.principal?.subject ?? 'anonymous'}</span>
        </div>
      </header>

      {error ? <div className="notice notice-error">{error}</div> : null}
      {loading ? <div className="notice">{copy.status}</div> : null}

      <section className="panel-grid">
        <Panel title="Supplier Profile" subtitle="Update supplier name and settings">
          <FormGrid>
            <input value={profileName} onChange={(e) => setProfileName(e.target.value)} placeholder="Supplier name" />
            <input value={profileStatus} onChange={(e) => setProfileStatus(e.target.value)} placeholder="status" />
            <textarea value={profileSettings} onChange={(e) => setProfileSettings(e.target.value)} rows={3} placeholder="JSON settings" />
            <button onClick={() => void submitProfile()}>Save profile</button>
          </FormGrid>
          {supplier ? <div className="hint">{supplier.code}</div> : null}
        </Panel>
        <Panel title="Supplier Markets">
          <Stack>{markets.map((market) => <Row key={market.id} title={market.market_code} meta={market.id.slice(0, 8)} status={market.status} />)}</Stack>
        </Panel>
      </section>

      <section className="panel-grid">
        <Panel title="Fulfillment Locations">
          <Stack>{locations.map((location) => <Row key={location.id} title={location.name} meta={`${location.code} · ${location.market_code}`} status={location.status} />)}</Stack>
          <FormGrid>
            <input value={locationForm.supplier_market_id} onChange={(e) => setLocationForm({ ...locationForm, supplier_market_id: e.target.value })} placeholder="supplier market id" />
            <input value={locationForm.market_code} onChange={(e) => setLocationForm({ ...locationForm, market_code: e.target.value })} placeholder="market code" />
            <input value={locationForm.code} onChange={(e) => setLocationForm({ ...locationForm, code: e.target.value })} placeholder="code" />
            <input value={locationForm.name} onChange={(e) => setLocationForm({ ...locationForm, name: e.target.value })} placeholder="name" />
            <input value={locationForm.location_type} onChange={(e) => setLocationForm({ ...locationForm, location_type: e.target.value })} placeholder="location type" />
            <input value={locationForm.status} onChange={(e) => setLocationForm({ ...locationForm, status: e.target.value })} placeholder="status" />
            <button onClick={() => void submitLocation()}>Create location</button>
          </FormGrid>
        </Panel>

        <Panel title="Products">
          <Stack>{products.map((product) => <Row key={product.id} title={product.slug} meta={product.id.slice(0, 8)} status={product.status} />)}</Stack>
          <FormGrid>
            <input value={productForm.slug} onChange={(e) => setProductForm({ ...productForm, slug: e.target.value })} placeholder="slug" />
            <input value={productForm.supplier_code} onChange={(e) => setProductForm({ ...productForm, supplier_code: e.target.value })} placeholder="supplier code" />
            <input value={productForm.status} onChange={(e) => setProductForm({ ...productForm, status: e.target.value })} placeholder="status" />
            <input value={productForm.ar_name} onChange={(e) => setProductForm({ ...productForm, ar_name: e.target.value })} placeholder="Arabic name" />
            <input value={productForm.en_name} onChange={(e) => setProductForm({ ...productForm, en_name: e.target.value })} placeholder="English name" />
            <button onClick={() => void submitProduct()}>Create product</button>
          </FormGrid>
        </Panel>
      </section>

      <section className="panel-grid">
        <Panel title="Offers">
          <Stack>{offers.map((offer) => <Row key={offer.id} title={offer.id} meta={`${offer.market_code} · ${offer.supplier_code ?? 'offer'}`} status={offer.status} />)}</Stack>
          <FormGrid>
            <input value={offerForm.supplier_product_id} onChange={(e) => setOfferForm({ ...offerForm, supplier_product_id: e.target.value })} placeholder="supplier product id" />
            <input value={offerForm.supplier_market_id} onChange={(e) => setOfferForm({ ...offerForm, supplier_market_id: e.target.value })} placeholder="supplier market id" />
            <input value={offerForm.market_code} onChange={(e) => setOfferForm({ ...offerForm, market_code: e.target.value })} placeholder="market code" />
            <input type="number" value={offerForm.amount_minor} onChange={(e) => setOfferForm({ ...offerForm, amount_minor: Number(e.target.value) })} placeholder="amount minor" />
            <input value={offerForm.currency} onChange={(e) => setOfferForm({ ...offerForm, currency: e.target.value })} placeholder="currency" />
            <input value={offerForm.status} onChange={(e) => setOfferForm({ ...offerForm, status: e.target.value })} placeholder="status" />
            <input type="number" value={offerForm.available_qty} onChange={(e) => setOfferForm({ ...offerForm, available_qty: Number(e.target.value) })} placeholder="available qty" />
            <label><input type="checkbox" checked={offerForm.is_available} onChange={(e) => setOfferForm({ ...offerForm, is_available: e.target.checked })} /> available</label>
            <button onClick={() => void submitOffer()}>Create offer</button>
          </FormGrid>
        </Panel>

        <Panel title="Inventory">
          <Stack>{snapshots.map((snapshot) => <Row key={snapshot.id} title={snapshot.id} meta={`on hand ${snapshot.on_hand_qty} · reserved ${snapshot.reserved_qty}`} status={`v${snapshot.version}`} />)}</Stack>
          <FormGrid>
            <input value={inventoryForm.fulfillment_location_id} onChange={(e) => setInventoryForm({ ...inventoryForm, fulfillment_location_id: e.target.value })} placeholder="location id" />
            <input value={inventoryForm.sku_id} onChange={(e) => setInventoryForm({ ...inventoryForm, sku_id: e.target.value })} placeholder="sku id" />
            <input type="number" value={inventoryForm.on_hand_qty} onChange={(e) => setInventoryForm({ ...inventoryForm, on_hand_qty: Number(e.target.value) })} placeholder="on hand qty" />
            <input value={inventoryForm.movement_type} onChange={(e) => setInventoryForm({ ...inventoryForm, movement_type: e.target.value })} placeholder="movement type" />
            <input type="number" value={inventoryForm.quantity_delta} onChange={(e) => setInventoryForm({ ...inventoryForm, quantity_delta: Number(e.target.value) })} placeholder="delta" />
            <input value={inventoryForm.reason} onChange={(e) => setInventoryForm({ ...inventoryForm, reason: e.target.value })} placeholder="reason" />
            <button onClick={() => void submitInventory()}>Create snapshot</button>
          </FormGrid>
          {snapshots[0] ? <button onClick={() => void adjustInventory(snapshots[0].id)}>Apply adjustment to first snapshot</button> : null}
          <FormGrid>
            <input value={selectedSnapshot} onChange={(e) => setSelectedSnapshot(e.target.value)} placeholder="snapshot id for movements" />
          </FormGrid>
          {selectedSnapshot ? <button onClick={() => void api.get(`/v1/supplier/inventory/${selectedSnapshot}/movements?locale=${locale}`).then((res) => res.json()).then((data: { items: Movement[] }) => setMovements(data.items))}>Load movements</button> : null}
          <Stack>{movements.map((movement) => <Row key={movement.id} title={movement.movement_type} meta={`${movement.quantity_delta} · ${movement.created_at}`} status={`${movement.on_hand_qty}/${movement.reserved_qty}`} />)}</Stack>
        </Panel>
      </section>
    </main>
  );
}

function Panel({ title, subtitle, children }: { title: string; subtitle?: string; children: React.ReactNode }) {
  return <section className="panel"><div className="panel-head"><div><h2>{title}</h2>{subtitle ? <p>{subtitle}</p> : null}</div></div>{children}</section>;
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
