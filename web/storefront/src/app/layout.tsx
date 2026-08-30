import type { Metadata } from 'next';
import '../styles.css';

export const metadata: Metadata = {
  title: 'Storefront',
  description: 'Seller storefront foundation'
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en" dir="ltr">
      <body>{children}</body>
    </html>
  );
}
