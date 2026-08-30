export const locales = ['ar', 'en'] as const;
export type Locale = (typeof locales)[number];

export const messages: Record<Locale, { appName: string; status: string }> = {
  ar: {
    appName: 'واجهة المتجر',
    status: 'جاهز للبناء'
  },
  en: {
    appName: 'Storefront',
    status: 'Ready for build'
  }
};

export function directionFor(locale: Locale): 'rtl' | 'ltr' {
  return locale === 'ar' ? 'rtl' : 'ltr';
}
