<?php
/**
 * Plugin Name: WP 3D Asset Editor (Babylon + React)
 * Description: Gutenberg block with a Babylon.js editor to place/rotate 3D assets; stores lat/lng/rotation as post meta.
 * Version: 0.1.1
 * Author: EduardoGDGV
 * Requires at least: 6.0
 * Requires PHP: 7.4
 */

if ( ! defined( 'ABSPATH' ) ) {
	exit; // Exit if accessed directly.
}

// Define plugin paths
define( 'WP3D_PLUGIN_DIR', plugin_dir_path( __FILE__ ) );
define( 'WP3D_PLUGIN_URL', plugin_dir_url( __FILE__ ) );

// Debug
error_log( 'WP3D_PLUGIN_DIR: ' . WP3D_PLUGIN_DIR );
error_log( 'WP3D_PLUGIN_URL: ' . WP3D_PLUGIN_URL );

// --- Include CPT and Meta registration files ---
require_once WP3D_PLUGIN_DIR . 'includes/register-cpt.php';
require_once WP3D_PLUGIN_DIR . 'includes/register-meta.php';

/**
 * Registers blocks from the build folder using the blocks manifest.
 * Runs early so CPT templates can reference blocks.
 */
function wp3d_register_blocks() {
	$build_dir     = WP3D_PLUGIN_DIR . 'build';
	$manifest_file = $build_dir . '/blocks-manifest.php';

	error_log( 'Checking build directory: ' . $build_dir );
	error_log( 'Checking manifest file: ' . $manifest_file );

	// WP 6.8+ optimized loader
	if ( function_exists( 'wp_register_block_types_from_metadata_collection' ) ) {
		if ( file_exists( $manifest_file ) ) {
			error_log( 'Using wp_register_block_types_from_metadata_collection' );
			wp_register_block_types_from_metadata_collection( $build_dir, $manifest_file );
		} else {
			error_log( 'Manifest file missing for WP 6.8+ loader' );
		}
		return;
	}

	// WP 6.7 loader
	if ( function_exists( 'wp_register_block_metadata_collection' ) ) {
		if ( file_exists( $manifest_file ) ) {
			error_log( 'Using wp_register_block_metadata_collection' );
			wp_register_block_metadata_collection( $build_dir, $manifest_file );
		} else {
			error_log( 'Manifest file missing for WP 6.7 loader' );
		}
		return;
	}

	// Fallback: manually register each block from manifest
	if ( file_exists( $manifest_file ) ) {
		$manifest_data = require $manifest_file;
		if ( is_array( $manifest_data ) ) {
			error_log( 'Manifest loaded, block types: ' . implode( ', ', array_keys( $manifest_data ) ) );
			foreach ( array_keys( $manifest_data ) as $block_type ) {
				$block_path = $build_dir . '/' . $block_type;
				if ( is_dir( $block_path ) && file_exists( $block_path . '/block.json' ) ) {
					register_block_type( $block_path );
					error_log( 'Registered block: ' . $block_type );
				} else {
					error_log( 'Skipped block (missing folder or block.json): ' . $block_type );
				}
			}
		}
	} else {
		error_log( 'Manifest file not found, no blocks registered' );
	}
}

// Register blocks early so CPT templates can use them
add_action( 'init', 'wp3d_register_blocks', 5 );
