<?php
// This file is generated. Do not modify it manually.
return array(
	'wp3d/asset-editor' => array(
		'$schema' => 'https://schemas.wp.org/trunk/block.json',
		'apiVersion' => 3,
		'name' => 'wp3d/asset-editor',
		'version' => '0.1.1',
		'title' => '3D Asset Editor',
		'category' => 'common',
		'icon' => 'location-alt',
		'description' => 'Place & rotate a 3D model; stores lat/lng/rotation in post meta.',
		'textdomain' => 'wp3d-asset-editor',
		'example' => array(
			
		),
		'supports' => array(
			'html' => false
		),
		'attributes' => array(
			'modelUrl' => array(
				'type' => 'string',
				'source' => 'meta',
				'meta' => 'asset_model_url'
			),
			'lat' => array(
				'type' => 'number',
				'source' => 'meta',
				'meta' => 'asset_lat'
			),
			'lng' => array(
				'type' => 'number',
				'source' => 'meta',
				'meta' => 'asset_lng'
			),
			'yaw' => array(
				'type' => 'number',
				'source' => 'meta',
				'meta' => 'asset_yaw'
			),
			'pitch' => array(
				'type' => 'number',
				'source' => 'meta',
				'meta' => 'asset_pitch'
			),
			'roll' => array(
				'type' => 'number',
				'source' => 'meta',
				'meta' => 'asset_roll'
			)
		),
		'editorScript' => 'file:./index.js',
		'editorStyle' => 'file:./style-index.css',
		'style' => 'file:./style-index.css'
	)
);
