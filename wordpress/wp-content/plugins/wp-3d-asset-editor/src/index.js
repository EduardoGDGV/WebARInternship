import { registerBlockType } from '@wordpress/blocks';
import './style.css';
import Edit from './edit';
import metadata from './block.json';

registerBlockType(metadata.name, {
  edit: Edit,
  save: () => null, // dynamic block — handled by render.php
});
