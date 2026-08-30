import { directionFor, messages, type Locale } from '../i18n/locales';

export default async function Home({
  searchParams
}: {
  searchParams?: Promise<{ locale?: string }>;
}) {
  const params = await searchParams;
  const locale: Locale = params?.locale === 'ar' ? 'ar' : 'en';
  const copy = messages[locale];

  return (
    <main className="shell" lang={locale} dir={directionFor(locale)}>
      <h1>{copy.appName}</h1>
      <p>{copy.status}</p>
    </main>
  );
}
