import clsx from 'clsx';
import { h } from 'preact';
import styles from './auth.module.css';

export function Auth() {
  return (
    <div className={clsx('auth', styles.root)}>
      <label>Please sign in to comment</label>{' '}
    </div>
  );
}
